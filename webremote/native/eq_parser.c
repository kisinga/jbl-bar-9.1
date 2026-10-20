/* Owner-built parser helper. Never calls Process, SetParam, InitEffect or watcher.
 * Caller must hash-pin libhdsp and its dependency closure before invocation.
 * CLI modes: LIB defaults OUT | LIB registry OUT | LIB parse JSON OUT.
 * No device operations or writes beyond explicitly supplied output paths. */
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <dlfcn.h>
#include <unistd.h>
#define BLOCK_SIZE 3840u
#define MAX_JSON 65536u
static void fail(const char *why) { fprintf(stderr,"eq-parser: %s\n",why); _exit(2); }
static void write_blob(const char *path,const void *data,size_t n) {
 FILE *f=fopen(path,"wb");
 if(!f || fwrite(data,1,n,f)!=n || fclose(f)) fail("output write failed");
}
static void json_string(FILE *f,const char *s) {
 fputc('"',f);
 for(size_t i=0;;i++) {
  unsigned char c=(unsigned char)s[i];
  if(i>128) fail("registry string exceeds bound");
  if(!c) break;
  if(c=='"' || c=='\\') { fputc('\\',f); fputc(c,f); }
  else if(c<32 || c>126) fprintf(f,"\\u%04x",(unsigned)c);
  else fputc(c,f);
 }
 fputc('"',f);
}
static void registry(unsigned char *base,const char *path) {
 uint32_t *v=(void*)(base+0x20526c);
 uint32_t start=v[0],end=v[1];
 if(!start || end<start || (end-start)%20 || (end-start)>128*20) fail("invalid registry bounds");
 FILE *f=fopen(path,"wb");if(!f) fail("registry output open failed");
 fputs("[\n",f);int first=1;
 for(uint32_t *e=(void*)(uintptr_t)start;e<(uint32_t*)(uintptr_t)end;e+=5) {
  if(e[3]>=BLOCK_SIZE || e[4]>128) fail("invalid registry entry");
  if(!first) fputs(",\n",f);
  first=0;
  fputs(" {\"effect\":",f);json_string(f,(const char*)(uintptr_t)e[0]);
  fputs(",\"name\":",f);json_string(f,(const char*)(uintptr_t)e[1]);
  fprintf(f,",\"version\":%u,\"offset\":%u,\"count\":%u}",e[2],e[3],e[4]);
 }
 fputs("\n]\n",f);if(ferror(f)||fclose(f))fail("registry output write failed");
}
int main(int argc,char **argv) {
 if(argc<4 || argc>5) fail("usage: LIB defaults OUT | LIB registry OUT | LIB parse JSON OUT");
 int is_parse=!strcmp(argv[2],"parse");
 if((is_parse && argc!=5) || (!is_parse && argc!=4)) fail("invalid arguments");
 if(!is_parse && strcmp(argv[2],"defaults") && strcmp(argv[2],"registry")) fail("invalid mode");
 void *h=dlopen(argv[1],RTLD_NOW|RTLD_LOCAL);if(!h)fail("pinned library load failed");
 Dl_info di;void *sym=dlsym(h,"GetParam");if(!sym || !dladdr(sym,&di))fail("missing library identity");
 unsigned char *base=di.dli_fbase;
 if(!strcmp(argv[2],"defaults")) {write_blob(argv[3],base+0x1c6bc0,BLOCK_SIZE);_exit(0);}
 if(!strcmp(argv[2],"registry")) {
  /* The immutable registry is initialized lazily by the parser. */
  int(*parse_registry)(const char*,void*)=(void*)(base+0x3c1dc);
  unsigned char ignored[BLOCK_SIZE];
  if(parse_registry("{\"process_chain\":[]}",ignored)!=1)fail("registry initialization failed");
  registry(base,argv[3]);_exit(0);
 }
 FILE *f=fopen(argv[3],"rb");if(!f)fail("input open failed");
 if(fseek(f,0,SEEK_END))fail("input size failed");
 long n=ftell(f);
 if(n<=0 || n>(long)MAX_JSON || fseek(f,0,SEEK_SET))fail("input exceeds bound");
 char *s=calloc(1,(size_t)n+1);if(!s || fread(s,1,(size_t)n,f)!=(size_t)n || fclose(f))fail("input read failed");
 if(memchr(s,0,(size_t)n))fail("embedded NUL in JSON");
 int(*parse)(const char*,void*)=(void*)(base+0x3c1dc);
 unsigned char block[BLOCK_SIZE];memset(block,0xa5,sizeof(block));
 if(parse(s,block)!=1)fail("native parse rejected JSON");
 write_blob(argv[4],block,sizeof(block));fflush(NULL);_exit(0);
}
