import ghidra.app.script.GhidraScript;
import ghidra.app.decompiler.*;
import ghidra.program.model.address.*;
import ghidra.program.model.listing.*;
import ghidra.program.model.scalar.*;
import java.util.*;

public class FindMovwRefs extends GhidraScript {
    public void run() throws Exception {
        String[] args = getScriptArgs();
        long target = Long.parseLong(args[0], 16);
        int low = (int)(target & 0xFFFF);
        int high = (int)((target >> 16) & 0xFFFF);
        Listing listing = currentProgram.getListing();
        InstructionIterator it = listing.getInstructions(true);
        DecompInterface ifc = new DecompInterface();
        ifc.openProgram(currentProgram);
        Set<String> done = new HashSet<String>();
        while (it.hasNext()) {
            Instruction ins = it.next();
            String mn = ins.getMnemonicString().toLowerCase();
            if (!mn.equals("movw")) continue;
            Scalar s = null;
            for (Object o : ins.getOpObjects(1)) if (o instanceof Scalar) s = (Scalar)o;
            if (s == null) continue;
            if ((s.getUnsignedValue() & 0xFFFF) != low) continue;
            // check next 3 instructions for matching movt
            Instruction n = ins;
            boolean ok = false;
            for (int i = 0; i < 3; i++) {
                n = n.getNext();
                if (n == null) break;
                if (n.getMnemonicString().toLowerCase().equals("movt")) {
                    Scalar s2 = null;
                    for (Object o : n.getOpObjects(1)) if (o instanceof Scalar) s2 = (Scalar)o;
                    if (s2 != null && (s2.getUnsignedValue() & 0xFFFF) == high) { ok = true; break; }
                }
            }
            if (!ok) continue;
            Function f = currentProgram.getFunctionManager().getFunctionContaining(ins.getAddress());
            println("HIT @ " + ins.getAddress() + (f != null ? " in " + f.getName() + " @ " + f.getEntryPoint() : " (no function)"));
            if (f != null && done.add(f.getEntryPoint().toString())) {
                println("\n===== " + f.getName() + " @ " + f.getEntryPoint() + " =====");
                DecompileResults res = ifc.decompileFunction(f, 240, monitor);
                if (res.decompileCompleted()) println(res.getDecompiledFunction().getC());
                else println("DECOMPILE FAILED: " + res.getErrorMessage());
            }
        }
    }
}
