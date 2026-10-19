import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.*;
import ghidra.program.model.listing.*;

public class FuncRange extends GhidraScript {
    public void run() throws Exception {
        String[] args = getScriptArgs();
        long lo = Long.parseLong(args[0], 16) + 0x10000;
        long hi = Long.parseLong(args[1], 16) + 0x10000;
        AddressSpace sp = currentProgram.getAddressFactory().getDefaultAddressSpace();
        FunctionManager fm = currentProgram.getFunctionManager();
        FunctionIterator it = fm.getFunctions(sp.getAddress(lo), true);
        while (it.hasNext()) {
            Function f = it.next();
            long e = f.getEntryPoint().getOffset();
            if (e > hi) break;
            println(String.format("%x size=%d", e - 0x10000, f.getBody().getNumAddresses()));
        }
    }
}
