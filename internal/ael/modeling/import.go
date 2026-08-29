package modeling

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type svdDevice struct {
	Peripherals []svdPeripheral `xml:"peripherals>peripheral"`
}
type svdPeripheral struct {
	Registers []svdRegister `xml:"registers>register"`
}
type svdRegister struct {
	Name   string     `xml:"name"`
	Offset string     `xml:"addressOffset"`
	Size   string     `xml:"size"`
	Reset  string     `xml:"resetValue"`
	Fields []svdField `xml:"fields>field"`
}
type svdField struct {
	Name   string `xml:"name"`
	Offset string `xml:"bitOffset"`
	Width  string `xml:"bitWidth"`
	Access string `xml:"access"`
}

func ImportSVD(path, name string) (IR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return IR{}, err
	}
	var device svdDevice
	if err := xml.Unmarshal(data, &device); err != nil {
		return IR{}, err
	}
	ir := IR{APIVersion: APIVersion, Kind: "HardwareBehaviorIR", Name: name, BusWidth: 32, Timing: map[string]UnitValue{}, Grounding: map[string][]string{}}
	for _, peripheral := range device.Peripherals {
		for _, source := range peripheral.Registers {
			offset, err := parseInteger(source.Offset, 0)
			if err != nil {
				return IR{}, fmt.Errorf("register %s offset: %w", source.Name, err)
			}
			width, err := parseInteger(source.Size, 32)
			if err != nil {
				return IR{}, fmt.Errorf("register %s width: %w", source.Name, err)
			}
			if width != 8 && width != 16 && width != 32 && width != 64 {
				width = 32
			}
			reset, err := parseInteger(source.Reset, 0)
			if err != nil {
				return IR{}, fmt.Errorf("register %s reset: %w", source.Name, err)
			}
			register := Register{Name: source.Name, Offset: offset, Width: int(width), Reset: reset}
			for _, sourceField := range source.Fields {
				lsb, err := parseInteger(sourceField.Offset, 0)
				if err != nil {
					return IR{}, err
				}
				fieldWidth, err := parseInteger(sourceField.Width, 1)
				if err != nil {
					return IR{}, err
				}
				access := map[string]string{"read-only": "ro", "write-only": "wo", "read-write": "rw", "read-writeOnce": "rw", "writeOnce": "wo"}[sourceField.Access]
				if access == "" {
					access = "rw"
				}
				register.Fields = append(register.Fields, Field{Name: sourceField.Name, LSB: int(lsb), Width: int(fieldWidth), Access: access})
			}
			ir.Registers = append(ir.Registers, register)
			if end := offset + width/8; end > ir.Size {
				ir.Size = end
			}
		}
	}
	if len(ir.Registers) == 0 {
		return IR{}, errors.New("SVD contains no registers")
	}
	if ir.Size < 4 {
		ir.Size = 4
	}
	return ir, ir.Validate()
}

var (
	rdlRegisterPattern = regexp.MustCompile(`(?s)reg\s*\{(.*?)\}\s*([A-Za-z_][A-Za-z0-9_]*)\s*@\s*(0x[0-9A-Fa-f]+|[0-9]+)\s*;`)
	rdlFieldPattern    = regexp.MustCompile(`(?s)field\s*\{(.*?)\}\s*([A-Za-z_][A-Za-z0-9_]*)\s*\[\s*([0-9]+)\s*:\s*([0-9]+)\s*\]\s*;`)
	rdlSWPattern       = regexp.MustCompile(`\bsw\s*=\s*([A-Za-z/]+)\s*;`)
	rdlResetPattern    = regexp.MustCompile(`\breset\s*=\s*(0x[0-9A-Fa-f]+|[0-9]+)\s*;`)
)

func ImportSystemRDL(path, name string) (IR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return IR{}, err
	}
	ir := IR{APIVersion: APIVersion, Kind: "HardwareBehaviorIR", Name: name, BusWidth: 32, Timing: map[string]UnitValue{}, Grounding: map[string][]string{}}
	for _, match := range rdlRegisterPattern.FindAllStringSubmatch(string(data), -1) {
		offset, _ := strconv.ParseUint(match[3], 0, 64)
		register := Register{Name: match[2], Offset: offset, Width: 32}
		for _, fieldMatch := range rdlFieldPattern.FindAllStringSubmatch(match[1], -1) {
			msb, _ := strconv.Atoi(fieldMatch[3])
			lsb, _ := strconv.Atoi(fieldMatch[4])
			if msb < lsb {
				msb, lsb = lsb, msb
			}
			access := "rw"
			if sw := rdlSWPattern.FindStringSubmatch(fieldMatch[1]); sw != nil {
				access = map[string]string{"r": "ro", "w": "wo", "rw": "rw", "r/w": "rw"}[sw[1]]
				if access == "" {
					access = "rw"
				}
			}
			reset := uint64(0)
			if value := rdlResetPattern.FindStringSubmatch(fieldMatch[1]); value != nil {
				reset, _ = strconv.ParseUint(value[1], 0, 64)
			}
			register.Fields = append(register.Fields, Field{Name: fieldMatch[2], LSB: lsb, Width: msb - lsb + 1, Access: access, Reset: reset})
			register.Reset |= reset << lsb
		}
		ir.Registers = append(ir.Registers, register)
		if end := offset + 4; end > ir.Size {
			ir.Size = end
		}
	}
	if len(ir.Registers) == 0 {
		return IR{}, errors.New("SystemRDL contains no supported registers")
	}
	if ir.Size < 4 {
		ir.Size = 4
	}
	return ir, ir.Validate()
}

func parseInteger(value string, fallback uint64) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseUint(value, 0, 64)
}
