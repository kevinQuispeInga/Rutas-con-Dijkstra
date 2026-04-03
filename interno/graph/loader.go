package graph

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

func LoadGraphFromCSV(nodesPath, edgesPath string) (*Graph, error) {
	g := NewGraph()

	// --- NODOS ---
	nodesFile, err := os.Open(nodesPath)
	if err != nil {
		return nil, fmt.Errorf("abriendo nodes csv: %w", err)
	}
	defer nodesFile.Close()

	nodesReader := csv.NewReader(nodesFile)

	// saltar cabecera
	if _, err := nodesReader.Read(); err != nil {
		return nil, fmt.Errorf("leyendo cabecera nodes: %w", err)
	}

	for {
		rec, err := nodesReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("leyendo node: %w", err)
		}
		if len(rec) < 3 {
			continue
		}

		idInt, err := strconv.ParseInt(rec[0], 10, 64)
		if err != nil {
			continue
		}
		lat, err := strconv.ParseFloat(rec[1], 64)
		if err != nil {
			continue
		}
		lon, err := strconv.ParseFloat(rec[2], 64)
		if err != nil {
			continue
		}

		g.AddNode(NodeID(idInt), lat, lon)
	}

	// --- ARISTAS ---
	edgesFile, err := os.Open(edgesPath)
	if err != nil {
		return nil, fmt.Errorf("abriendo edges csv: %w", err)
	}
	defer edgesFile.Close()

	edgesReader := csv.NewReader(edgesFile)

	// saltar cabecera
	if _, err := edgesReader.Read(); err != nil {
		return nil, fmt.Errorf("leyendo cabecera edges: %w", err)
	}

	for {
		rec, err := edgesReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("leyendo edge: %w", err)
		}
		if len(rec) < 4 {
			continue
		}

		fromInt, err := strconv.ParseInt(rec[0], 10, 64)
		if err != nil {
			continue
		}
		toInt, err := strconv.ParseInt(rec[1], 10, 64)
		if err != nil {
			continue
		}
		dist, err := strconv.ParseFloat(rec[2], 64)
		if err != nil {
			continue
		}
		street := rec[3]

		from := NodeID(fromInt)
		to := NodeID(toInt)

		// si quieres grafo NO dirigido, agregas las dos direcciones:
		g.AddEdge(from, to, dist, street)
		g.AddEdge(to, from, dist, street)
	}

	return g, nil
}
