import ghidra.app.script.GhidraScript;
import ghidra.app.decompiler.*;
import ghidra.program.model.address.*;
import ghidra.program.model.listing.*;

public class DecompileMulti extends GhidraScript {
    public void run() throws Exception {
        String[] args = getScriptArgs();
        DecompInterface ifc = new DecompInterface();
        ifc.openProgram(currentProgram);
        AddressFactory af = currentProgram.getAddressFactory();
        for (String a : args) {
            long off = Long.parseLong(a, 16) + 0x10000;
            Address addr = af.getDefaultAddressSpace().getAddress(off);
            Function fn = currentProgram.getFunctionManager().getFunctionContaining(addr);
            if (fn == null) { println("NO FUNC for " + a); continue; }
            println("\n########## " + fn.getName() + " @ " + fn.getEntryPoint()
                    + " (queried " + a + ", size " + fn.getBody().getNumAddresses() + ") ##########");
            DecompileResults res = ifc.decompileFunction(fn, 240, monitor);
            if (res.decompileCompleted()) println(res.getDecompiledFunction().getC());
            else println("DECOMPILE FAILED: " + res.getErrorMessage());
        }
    }
}
