package fmi

import (
	"archive/zip"
	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"os"
	"path/filepath"
	"testing"
)

func TestSSPTopologyLoadsAndConvertsToAEL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.ssp")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("SystemStructure.ssd")
	_, _ = entry.Write([]byte(`<?xml version="1.0"?><ssd:SystemStructureDescription xmlns:ssd="http://ssp-standard.org/SSP1/SystemStructureDescription" version="1.0" name="five-domain"><ssd:System name="root"><ssd:Elements><ssd:Component name="source" source="source.fmu"><ssd:Connectors><ssd:Connector name="voltage" kind="output"><ssd:Real unit="V"/></ssd:Connector></ssd:Connectors></ssd:Component><ssd:Component name="sink" source="sink.fmu"><ssd:Connectors><ssd:Connector name="supply" kind="input"><ssd:Real unit="V"/></ssd:Connector></ssd:Connectors></ssd:Component></ssd:Elements><ssd:Connections><ssd:Connection startElement="source" startConnector="voltage" endElement="sink" endConnector="supply"/></ssd:Connections></ssd:System></ssd:SystemStructureDescription>`))
	_ = archive.Close()
	_ = file.Close()
	ssp, err := LoadSSP(path)
	if err != nil {
		t.Fatal(err)
	}
	system, err := ssp.System(map[string]ael.Backend{"source": ael.BackendNgspice, "sink": ael.BackendModelica}, map[string]string{"source": "source.fmu", "sink": "sink.fmu"})
	if err != nil {
		t.Fatal(err)
	}
	if len(system.Components) != 2 || len(system.Connections) != 1 || system.Connections[0].Unit != "V" {
		t.Fatalf("unexpected topology: %#v", system)
	}
}
