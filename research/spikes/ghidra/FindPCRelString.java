import ghidra.app.script.GhidraScript;
import ghidra.app.decompiler.*;
import ghidra.program.model.address.*;
import ghidra.program.model.listing.*;
import ghidra.program.model.scalar.*;
import ghidra.program.model.lang.*;
import ghidra.program.model.symbol.*;
import ghidra.program.model.mem.*;
import java.util.*;

public class FindPCRelString extends GhidraScript {
    public void run() throws Exception {
        String[] args = getScriptArgs();
        long target = Long.parseLong(args[0], 16);
        Memory mem = currentProgram.getMemory();
        byte[] pat = args.length > 1 ? args[1].getBytes("ASCII") : null;
        if (pat != null) {
            Address f = mem.findBytes(currentProgram.getMinAddress(), pat, null, true, monitor);
            if (f != null) { target = f.getOffset(); println("string @ " + f); }
        }
        Listing listing = currentProgram.getListing();
        Register pc = currentProgram.getProgramContext().getRegister("pc");
        Map<String,Long> regimm = new HashMap<String,Long>();
        Map<String,Long> regval = new HashMap<String,Long>();
        DecompInterface ifc = new DecompInterface();
        ifc.openProgram(currentProgram);
        Set<String> done = new HashSet<String>();
        InstructionIterator it = listing.getInstructions(true);
        while (it.hasNext()) {
            Instruction ins = it.next();
            String mn = ins.getMnemonicString().toLowerCase();
            if (mn.equals("movw") || mn.equals("movt")) {
                Object[] oo = ins.getOpObjects(0);
                Object[] oi = ins.getOpObjects(1);
                String rd = null; Long imm = null;
                for (Object o : oo) if (o instanceof Register) rd = ((Register)o).getName();
                for (Object o : oi) if (o instanceof Scalar) imm = ((Scalar)o).getUnsignedValue() & 0xFFFF;
                if (rd != null && imm != null) {
                    long cur = regimm.containsKey(rd) ? regimm.get(rd) : 0;
                    if (mn.equals("movw")) cur = (cur & 0xFFFF0000L) | imm;
                    else cur = (cur & 0xFFFFL) | (imm << 16);
                    regimm.put(rd, cur);
                }
                continue;
            }
            if (mn.startsWith("add")) {
                // look for add rd, rx, pc or add rd, pc, rx (ARM or Thumb)
                int nops = ins.getNumOperands();
                String rd = null, rx = null; boolean hasPc = false;
                for (int i = 0; i < nops; i++) {
                    for (Object o : ins.getOpObjects(i)) {
                        if (o instanceof Register) {
                            String rn = ((Register)o).getName();
                            if (rn.equalsIgnoreCase("pc")) hasPc = true;
                            else if (i == 0) rd = rn;
                            else rx = rn;
                        }
                    }
                }
                if (hasPc && rx != null && regimm.containsKey(rx)) {
                    boolean thumb = ins.getAddress().getOffset() % 2 == 0; // can't tell; try both
                    long a = ins.getAddress().getOffset();
                    a -= 0x10000; long pcArm = a + 8;
                    long pcThumb = (a & ~3L) + 4;
                    long v = regimm.get(rx);
                    if (pcArm + v == target || pcThumb + v == target) {
                        println("HIT @ " + ins.getAddress());
                        Function fn = currentProgram.getFunctionManager().getFunctionContaining(ins.getAddress());
                        if (fn != null && done.add(fn.getEntryPoint().toString())) {
                            println("\n===== " + fn.getName() + " @ " + fn.getEntryPoint() + " =====");
                            DecompileResults res = ifc.decompileFunction(fn, 240, monitor);
                            if (res.decompileCompleted()) println(res.getDecompiledFunction().getC());
                            else println("DECOMPILE FAILED: " + res.getErrorMessage());
                        }
                    }
                }
            }
        }
        if (done.isEmpty()) println("no hits");
    }
}
