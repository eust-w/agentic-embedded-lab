package modeling

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var identifierParts = regexp.MustCompile(`[^A-Za-z0-9]+`)

func GenerateRenodeCSharp(ir IR, namespace string) (string, error) {
	if err := ir.Validate(); err != nil {
		return "", err
	}
	if namespace == "" {
		namespace = "Ael.Generated"
	}
	className := ""
	for _, part := range identifierParts.Split(ir.Name, -1) {
		if part != "" {
			className += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	if className == "" {
		return "", fmt.Errorf("model name cannot produce a C# identifier")
	}
	registers := append([]Register(nil), ir.Registers...)
	sort.Slice(registers, func(i, j int) bool { return registers[i].Offset < registers[j].Offset })
	var definitions, declarations, enums []string
	for _, register := range registers {
		builder := fmt.Sprintf("            Registers.%s.Define(this, 0x%x)", register.Name, register.Reset)
		for _, field := range register.Fields {
			method := "WithValueField"
			fieldType := "IValueRegisterField"
			width := fmt.Sprintf(", %d", field.Width)
			if field.Width == 1 {
				method, fieldType, width = "WithFlag", "IFlagRegisterField", ""
			}
			mode := map[string]string{"ro": "FieldMode.Read", "wo": "FieldMode.Write", "rw": "FieldMode.Read | FieldMode.Write", "w1c": "FieldMode.Read | FieldMode.WriteOneToClear", "w1s": "FieldMode.Read | FieldMode.Set"}[field.Access]
			variable := strings.ToLower(register.Name[:1]) + register.Name[1:] + strings.ToUpper(field.Name[:1]) + field.Name[1:]
			builder += fmt.Sprintf("\n                .%s(%d%s, out %s, mode: %s, name: \"%s\")", method, field.LSB, width, variable, mode, field.Name)
			declarations = append(declarations, fmt.Sprintf("        private %s %s;", fieldType, variable))
		}
		definitions = append(definitions, builder+";")
		enums = append(enums, fmt.Sprintf("            %s = 0x%x,", register.Name, register.Offset))
	}
	return fmt.Sprintf(`// SPDX-License-Identifier: Apache-2.0
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Peripherals.Bus;

namespace %s
{
    public sealed class %s : BasicDoubleWordPeripheral, IKnownSize
    {
        public %s(IMachine machine) : base(machine)
        {
%s
        }

        public long Size => 0x%x;

%s

        private enum Registers : long
        {
%s
        }
    }
}
`, namespace, className, className, strings.Join(definitions, "\n"), ir.Size, strings.Join(declarations, "\n"), strings.Join(enums, "\n")), nil
}
