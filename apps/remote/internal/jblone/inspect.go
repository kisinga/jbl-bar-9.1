// Package jblone starts with read-only JBL One-family protocol inspection.
// Wire shapes are source-observed; physical firmware qualification is pending.
package jblone

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const soapNamespace = "http://schemas.xmlsoap.org/soap/envelope/"
const maxResponse = 128 * 1024

type query struct{ path, service, action string }

var queries = []query{
	{"/upnp/control/rendercontrol1", "RenderingControl", "GetControlDeviceInfo"},
	{"/upnp/control/rendertransport1", "AVTransport", "GetInfoEx"},
}

// Client has no arbitrary-command entry point, scan, credentials or setters.
type Client struct {
	origin string
	http   *http.Client
}

func NewClient(host string) (*Client, error) {
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Is4() || !ip.IsPrivate() {
		return nil, errors.New("use the soundbar's private IPv4 address, without a URL or port")
	}
	transport := &http.Transport{
		// Do not send LAN requests through environment proxies.
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 16 * 1024,
	}
	return &Client{
		origin: "http://" + net.JoinHostPort(ip.String(), "59152"),
		http: &http.Client{Transport: transport, Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

type Observation struct {
	Action         string            `json:"action"`
	URL            string            `json:"url"`
	Request        string            `json:"request_xml"`
	ObservedAt     time.Time         `json:"observed_at"`
	HTTPStatus     int               `json:"http_status,omitempty"`
	Response       string            `json:"response_xml,omitempty"`
	ResponseBase64 string            `json:"response_bytes_base64,omitempty"`
	Truncated      bool              `json:"truncated,omitempty"`
	Fields         map[string]string `json:"fields,omitempty"`
	Error          string            `json:"error,omitempty"`
}

type Report struct {
	Schema       int               `json:"schema"`
	Scope        string            `json:"scope"`
	Host         string            `json:"host"`
	Identity     map[string]string `json:"observed_identity"`
	Observations []Observation     `json:"observations"`
}

// Inspect makes each of the two fixed getter requests once. Partial failures
// remain independent; an unreachable port is not an unsupported-model verdict.
func (c *Client) Inspect(ctx context.Context) Report {
	r := Report{Schema: 1, Scope: "experimental read-only protocol observations; not hardware qualification", Host: c.origin, Identity: map[string]string{}}
	for _, q := range queries {
		r.Observations = append(r.Observations, c.read(ctx, q))
	}
	info := r.Observations[0]
	if info.Error == "" {
		var values map[string]json.RawMessage
		if err := json.Unmarshal([]byte(info.Fields["Status"]), &values); err != nil || values == nil {
			r.Observations[0].Error = "device Status is not a JSON object"
		} else {
			for _, key := range []string{"hm_product_name", "hardware", "firmware"} {
				var value string
				if json.Unmarshal(values[key], &value) == nil && strings.TrimSpace(value) != "" {
					r.Identity[key] = value
				}
			}
		}
	}
	return r
}

func (c *Client) read(ctx context.Context, q query) Observation {
	service := "urn:schemas-upnp-org:service:" + q.service + ":1"
	body := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="` + soapNamespace + `" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + q.action + ` xmlns:u="` + service + `"><InstanceID>0</InstanceID></u:` + q.action + `></s:Body></s:Envelope>`
	o := Observation{Action: q.action, URL: c.origin + q.path, Request: body, ObservedAt: time.Now().UTC()}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, strings.NewReader(body))
	if err != nil {
		o.Error = "cannot construct getter request"
		return o
	}
	request.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	request.Header.Set("SOAPAction", `"`+service+"#"+q.action+`"`)
	response, err := c.http.Do(request)
	if err != nil {
		o.Error = "getter request failed or timed out; no retry was sent"
		return o
	}
	defer response.Body.Close()
	o.HTTPStatus = response.StatusCode
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if len(data) > maxResponse {
		o.Truncated = true
		data = data[:maxResponse]
	}
	if utf8.Valid(data) {
		o.Response = string(data)
	} else {
		// Preserve malformed or mid-codepoint truncated replies losslessly.
		o.ResponseBase64 = base64.StdEncoding.EncodeToString(data)
	}
	switch {
	case err != nil:
		o.Error = "response body could not be read completely"
	case o.Truncated:
		o.Error = "response exceeds 128 KiB; retained prefix only"
	case response.StatusCode != http.StatusOK:
		o.Error = fmt.Sprintf("getter returned HTTP %d", response.StatusCode)
	default:
		o.Fields, err = responseFields(data, service, q.action)
		if err != nil {
			o.Error = err.Error()
		}
	}
	return o
}

func responseFields(data []byte, service, action string) (map[string]string, error) {
	var envelope struct {
		XMLName xml.Name
		Bodies  []struct {
			Responses []struct {
				XMLName xml.Name
				Fields  []struct {
					XMLName xml.Name
					Value   string `xml:",chardata"`
				} `xml:",any"`
			} `xml:",any"`
		} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		return nil, errors.New("invalid SOAP XML")
	}
	// Decode must consume one complete document, including the closing Envelope.
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid trailing SOAP XML")
		}
		if text, ok := token.(xml.CharData); !ok || strings.TrimSpace(string(text)) != "" {
			return nil, errors.New("unexpected trailing SOAP XML")
		}
	}
	if envelope.XMLName != (xml.Name{Space: soapNamespace, Local: "Envelope"}) || len(envelope.Bodies) != 1 || len(envelope.Bodies[0].Responses) != 1 {
		return nil, errors.New("expected one SOAP Envelope, Body and response")
	}
	response := envelope.Bodies[0].Responses[0]
	if response.XMLName == (xml.Name{Space: soapNamespace, Local: "Fault"}) {
		return nil, errors.New("device returned a SOAP fault")
	}
	if response.XMLName != (xml.Name{Space: service, Local: action + "Response"}) {
		return nil, errors.New("SOAP response does not match the getter")
	}
	fields := map[string]string{}
	for _, field := range response.Fields {
		if _, duplicate := fields[field.XMLName.Local]; duplicate {
			return nil, errors.New("duplicate SOAP response field")
		}
		fields[field.XMLName.Local] = strings.TrimSpace(field.Value)
	}
	return fields, nil
}

// Fixture translates only directly observed volume/mute. Vendor source codes,
// power, Smart Mode and controls stay unknown pending physical comparisons.
// It is consumed by the existing read-only preview, not a new frontend.
func (r Report) Fixture() map[string]any {
	s := map[string]any{"available": false, "power": "Unknown", "mode": "Unknown", "input": "Unknown", "volume": nil, "muted": nil,
		"error": "Research snapshot; power, source semantics and controls remain unverified."}
	for _, o := range r.Observations {
		if o.Action != "GetInfoEx" || o.Error != "" {
			continue
		}
		if volume, err := strconv.ParseUint(o.Fields["CurrentVolume"], 10, 16); err == nil {
			s["volume"], s["available"] = volume, true
		}
		switch o.Fields["CurrentMute"] {
		case "0":
			s["muted"], s["available"] = false, true
		case "1":
			s["muted"], s["available"] = true, true
		}
	}
	return s
}
