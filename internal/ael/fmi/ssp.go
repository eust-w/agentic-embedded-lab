package fmi

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"io"
	"path/filepath"
	"time"
)

type SSP struct {
	Version     string
	Name        string
	Components  []SSPComponent
	Connections []SSPConnection
}
type SSPComponent struct {
	Name, Source string
	Connectors   []SSPConnector
}
type SSPConnector struct{ Name, Kind, Type, Unit string }
type SSPConnection struct{ StartElement, StartConnector, EndElement, EndConnector string }
type ssdDocument struct {
	Version string    `xml:"version,attr"`
	Name    string    `xml:"name,attr"`
	System  ssdSystem `xml:"System"`
}
type ssdSystem struct {
	Name        string          `xml:"name,attr"`
	Elements    []ssdComponent  `xml:"Elements>Component"`
	Connections []ssdConnection `xml:"Connections>Connection"`
}
type ssdComponent struct {
	Name       string         `xml:"name,attr"`
	Source     string         `xml:"source,attr"`
	Connectors []ssdConnector `xml:"Connectors>Connector"`
}
type ssdConnector struct {
	Name    string      `xml:"name,attr"`
	Kind    string      `xml:"kind,attr"`
	Real    *ssdReal    `xml:"Real"`
	Integer *ssdInteger `xml:"Integer"`
	Boolean *struct{}   `xml:"Boolean"`
	String  *struct{}   `xml:"String"`
}
type ssdReal struct {
	Unit string `xml:"unit,attr"`
}
type ssdInteger struct {
	Unit string `xml:"unit,attr"`
}
type ssdConnection struct {
	StartElement   string `xml:"startElement,attr"`
	StartConnector string `xml:"startConnector,attr"`
	EndElement     string `xml:"endElement,attr"`
	EndConnector   string `xml:"endConnector,attr"`
}

func LoadSSP(path string) (SSP, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return SSP{}, err
	}
	defer archive.Close()
	var reader io.ReadCloser
	for _, file := range archive.File {
		if filepath.ToSlash(file.Name) == "SystemStructure.ssd" {
			reader, err = file.Open()
			break
		}
	}
	if err != nil {
		return SSP{}, err
	}
	if reader == nil {
		return SSP{}, errors.New("SSP archive has no SystemStructure.ssd")
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, 4<<20))
	var document ssdDocument
	if err := decoder.Decode(&document); err != nil {
		return SSP{}, err
	}
	ssp := SSP{Version: document.Version, Name: document.Name}
	if ssp.Name == "" {
		ssp.Name = document.System.Name
	}
	for _, source := range document.System.Elements {
		component := SSPComponent{Name: source.Name, Source: source.Source}
		for _, connector := range source.Connectors {
			value := SSPConnector{Name: connector.Name, Kind: connector.Kind}
			switch {
			case connector.Real != nil:
				value.Type, value.Unit = "real", connector.Real.Unit
			case connector.Integer != nil:
				value.Type, value.Unit = "integer", connector.Integer.Unit
			case connector.Boolean != nil:
				value.Type = "boolean"
			case connector.String != nil:
				value.Type = "string"
			default:
				return SSP{}, fmt.Errorf("connector %s.%s has no supported type", source.Name, connector.Name)
			}
			component.Connectors = append(component.Connectors, value)
		}
		ssp.Components = append(ssp.Components, component)
	}
	for _, connection := range document.System.Connections {
		ssp.Connections = append(ssp.Connections, SSPConnection(connection))
	}
	return ssp, ssp.Validate()
}
func (s SSP) Validate() error {
	if s.Version != "1.0" && s.Version != "2.0" {
		return fmt.Errorf("unsupported SSP version %s", s.Version)
	}
	components := map[string]SSPComponent{}
	connectors := map[string]SSPConnector{}
	for _, component := range s.Components {
		if component.Name == "" || components[component.Name].Name != "" {
			return errors.New("SSP component names must be unique")
		}
		components[component.Name] = component
		for _, connector := range component.Connectors {
			key := component.Name + "." + connector.Name
			if connector.Name == "" || connectors[key].Name != "" {
				return fmt.Errorf("duplicate SSP connector %s", key)
			}
			if connector.Kind != "input" && connector.Kind != "output" && connector.Kind != "parameter" {
				return fmt.Errorf("unsupported SSP connector kind %s", connector.Kind)
			}
			connectors[key] = connector
		}
	}
	for _, connection := range s.Connections {
		source := connectors[connection.StartElement+"."+connection.StartConnector]
		target := connectors[connection.EndElement+"."+connection.EndConnector]
		if source.Name == "" || target.Name == "" {
			return errors.New("SSP connection references an unknown connector")
		}
		if source.Kind != "output" || target.Kind != "input" || source.Type != target.Type || source.Unit != target.Unit {
			return fmt.Errorf("incompatible SSP connection %s.%s", connection.StartElement, connection.StartConnector)
		}
	}
	return nil
}
func (s SSP) System(backends map[string]ael.Backend, models map[string]string) (ael.System, error) {
	if err := s.Validate(); err != nil {
		return ael.System{}, err
	}
	system := ael.System{APIVersion: ael.APIVersion, ID: s.Name}
	for _, component := range s.Components {
		backend := backends[component.Name]
		if backend == "" {
			return ael.System{}, fmt.Errorf("backend missing for SSP component %s", component.Name)
		}
		converted := ael.Component{ID: component.Name, Backend: backend, Model: models[component.Name], StepUS: 1000, Properties: map[string]any{}, Fidelity: ael.Fidelity{Firmware: ael.FidelityFunctional, Register: ael.FidelitySynthetic, Protocol: ael.FidelityFunctional, Timing: ael.FidelityFunctional, Physical: ael.FidelityUnsupported, Limitations: []string{"SSP topology import does not establish hardware equivalence"}}}
		for _, connector := range component.Connectors {
			if connector.Kind == "parameter" {
				continue
			}
			converted.Ports = append(converted.Ports, ael.Port{Name: connector.Name, Direction: connector.Kind, Type: connector.Type, Unit: connector.Unit})
		}
		system.Components = append(system.Components, converted)
	}
	for _, connection := range s.Connections {
		source := findConnector(s, connection.StartElement, connection.StartConnector)
		system.Connections = append(system.Connections, ael.Connection{SourceComponent: connection.StartElement, SourcePort: connection.StartConnector, TargetComponent: connection.EndElement, TargetPort: connection.EndConnector, Unit: source.Unit})
	}
	return system, ael.Validate(ael.Experiment{APIVersion: ael.APIVersion, ID: "ssp-validation", SystemID: system.ID, DurationUS: 1, MacroStepUS: 1, Timeout: time.Second}, system)
}
func findConnector(s SSP, component, name string) SSPConnector {
	for _, item := range s.Components {
		if item.Name != component {
			continue
		}
		for _, connector := range item.Connectors {
			if connector.Name == name {
				return connector
			}
		}
	}
	return SSPConnector{}
}
