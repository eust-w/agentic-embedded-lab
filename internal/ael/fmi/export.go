package fmi

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

var unsafeIdentifier = regexp.MustCompile(`[^A-Za-z0-9_]`)

func ProxyName(backend ael.Backend) (string, bool) {
	value, ok := map[ael.Backend]string{
		ael.BackendRenode: "RenodeFmu", ael.BackendNgspice: "NgspiceFmu",
		ael.BackendModelica: "ModelicaFmu", ael.BackendNS3: "Ns3Fmu",
		ael.BackendOpenEMS: "OpenEmsFmu",
	}[backend]
	return value, ok
}

func ExportSSP(system ael.System, fmus map[string]string, destination string) error {
	if len(system.Components) == 0 {
		return errors.New("SSP system has no components")
	}
	identifier := unsafeIdentifier.ReplaceAllString(system.ID, "_")
	if identifier == "" || identifier[0] >= '0' && identifier[0] <= '9' {
		identifier = "ael_" + identifier
	}
	var document bytes.Buffer
	document.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	fmt.Fprintf(&document, `<ssd:SystemStructureDescription xmlns:ssd="http://ssp-standard.org/SSP1/SystemStructureDescription" xmlns:ssc="http://ssp-standard.org/SSP1/SystemStructureCommon" xmlns:oms="https://raw.githubusercontent.com/OpenModelica/OMSimulator/master/schema/oms.xsd" version="1.0" name="%s"><ssd:System name="%s"><ssd:Elements>`, xmlEscape(identifier), xmlEscape(identifier))
	for _, component := range system.Components {
		fmu := fmus[component.ID]
		if fmu == "" {
			return fmt.Errorf("missing FMU for component %s", component.ID)
		}
		fmt.Fprintf(&document, `<ssd:Component name="%s" source="resources/%s"><ssd:Connectors>`, xmlEscape(component.ID), xmlEscape(filepath.Base(fmu)))
		for _, port := range component.Ports {
			typeName := map[string]string{"real": "Real", "integer": "Integer", "boolean": "Boolean", "string": "String"}[port.Type]
			if typeName == "" {
				return fmt.Errorf("unsupported SSP port type %s", port.Type)
			}
			fmt.Fprintf(&document, `<ssd:Connector name="%s" kind="%s"><ssc:%s`, xmlEscape(port.Name), xmlEscape(port.Direction), typeName)
			if port.Unit != "" && port.Unit != "1" {
				fmt.Fprintf(&document, ` unit="%s"`, xmlEscape(port.Unit))
			}
			document.WriteString(` /></ssd:Connector>`)
		}
		document.WriteString(`</ssd:Connectors></ssd:Component>`)
	}
	document.WriteString(`</ssd:Elements><ssd:Connections>`)
	for _, connection := range system.Connections {
		fmt.Fprintf(&document, `<ssd:Connection startElement="%s" startConnector="%s" endElement="%s" endConnector="%s" />`, xmlEscape(connection.SourceComponent), xmlEscape(connection.SourcePort), xmlEscape(connection.TargetComponent), xmlEscape(connection.TargetPort))
	}
	step := int64(0)
	for _, component := range system.Components {
		if component.StepUS > 0 {
			if step == 0 {
				step = component.StepUS
			} else {
				step = greatestCommonDivisor(step, component.StepUS)
			}
		}
	}
	if step == 0 {
		step = 1000
	}
	fmt.Fprintf(&document, `</ssd:Connections><ssd:Annotations><ssc:Annotation type="org.openmodelica"><oms:Annotations><oms:SimulationInformation><oms:FixedStepMaster description="oms-ma" stepSize="%.9f" absoluteTolerance="0.0001" relativeTolerance="0.0001" /></oms:SimulationInformation></oms:Annotations></ssc:Annotation></ssd:Annotations></ssd:System></ssd:SystemStructureDescription>`, float64(step)/1e6)

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	files := map[string][]byte{"SystemStructure.ssd": document.Bytes()}
	for component, path := range fmus {
		data, err := os.ReadFile(path)
		if err != nil {
			archive.Close()
			file.Close()
			return fmt.Errorf("read FMU %s: %w", component, err)
		}
		files["resources/"+filepath.Base(path)] = data
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Unix(0, 0).UTC())
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			file.Close()
			return err
		}
		if _, err := writer.Write(files[name]); err != nil {
			archive.Close()
			file.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func xmlEscape(value string) string {
	var output strings.Builder
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

func greatestCommonDivisor(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}
