package main

func KnownAROSBootRegressionInventory() map[string]string {
	return map[string]string{
		(M68KFaultRecord{
			Class:          M68KFaultClassIllegalInstruction,
			FaultPC:        0x00624910,
			Opcode:         0x226B,
			MnemonicFamily: "MOVEA",
			AddressingMode: "(d16,An)",
		}).Signature(): "TestM68K_MOVEAL_AddressDisplacementToAddressRegister",
	}
}
