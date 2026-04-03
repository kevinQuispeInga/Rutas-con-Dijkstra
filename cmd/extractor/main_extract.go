package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/qedus/osmpbf"
)

// Bounding box aproximado para Lima Metropolitana
// Ajusta estos valores si quieres más/menos área
const (
	minLatLima = -12.30
	maxLatLima = -11.90
	minLonLima = -77.20
	maxLonLima = -76.70
)

// Información básica de un nodo (coordenadas).
type NodoInfo struct {
	Lat float64
	Lon float64
}

// Arista para exportar a CSV.
type AristaCSV struct {
	From     int64
	To       int64
	Distance float64
	Street   string
}

// Punto de reciclaje.
type RecyclingPoint struct {
	ID   int64
	Lat  float64
	Lon  float64
	Name string
}

func main() {
	pbfPath := "./peru-251120.osm.pbf"
	outputDir := "./datos"

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("error creando carpeta datos: %v", err)
	}

	f, err := os.Open(pbfPath)
	if err != nil {
		log.Fatalf("error abriendo PBF %s: %v", pbfPath, err)
	}
	defer f.Close()

	log.Printf("Iniciando decodificación del PBF...")

	decoder := osmpbf.NewDecoder(f)
	if err := decoder.Start(1); err != nil {
		log.Fatalf("error iniciando decoder: %v", err)
	}

	// Todos los nodos (para saber lat/lon por ID).
	nodos := make(map[int64]NodoInfo)

	// Conjunto de nodos que realmente usaremos en Lima (los que aparecen en aristas dentro del bbox)
	nodosLima := make(map[int64]struct{})

	var aristasLima []AristaCSV
	var reciclajeLima []RecyclingPoint

	for {
		v, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("error decodificando PBF: %v", err)
		}

		switch v := v.(type) {
		case *osmpbf.Node:
			// Guardamos TODAS las coordenadas de nodos (luego filtramos por bbox)
			nodos[v.ID] = NodoInfo{
				Lat: v.Lat,
				Lon: v.Lon,
			}

			// ¿Es un punto de reciclaje dentro de Lima?
			if v.Tags != nil {
				amenity := v.Tags["amenity"]
				recType := v.Tags["recycling_type"]

				if amenity == "recycling" || recType != "" {
					if dentroDeLima(v.Lat, v.Lon) {
						name := v.Tags["name"]
						reciclajeLima = append(reciclajeLima, RecyclingPoint{
							ID:   v.ID,
							Lat:  v.Lat,
							Lon:  v.Lon,
							Name: name,
						})
					}
				}
			}

		case *osmpbf.Way:
			if v.Tags == nil {
				continue
			}

			// Solo nos interesan las calles.
			highway := v.Tags["highway"]
			if highway == "" {
				continue
			}

			streetName := v.Tags["name"]
			refs := v.NodeIDs
			if len(refs) < 2 {
				continue
			}

			// Recorremos pares consecutivos de nodos de la vía.
			for i := 0; i < len(refs)-1; i++ {
				id1 := refs[i]
				id2 := refs[i+1]

				n1, ok1 := nodos[id1]
				n2, ok2 := nodos[id2]
				if !ok1 || !ok2 {
					continue
				}

				// Ambos nodos deben caer dentro del bbox de Lima
				if !dentroDeLima(n1.Lat, n1.Lon) || !dentroDeLima(n2.Lat, n2.Lon) {
					continue
				}

				dist := haversine(n1.Lat, n1.Lon, n2.Lat, n2.Lon)

				aristasLima = append(aristasLima, AristaCSV{
					From:     id1,
					To:       id2,
					Distance: dist,
					Street:   streetName,
				})
				nodosLima[id1] = struct{}{}
				nodosLima[id2] = struct{}{}
			}
		}
	}

	log.Printf("Decodificación completa. Escribiendo CSV de Lima...")

	if err := escribirCSVLima(outputDir, nodos, nodosLima, aristasLima, reciclajeLima); err != nil {
		log.Fatalf("error escribiendo CSV de Lima: %v", err)
	}

	log.Println("Listo. CSV de Lima generados en la carpeta 'datos'.")
}

// --------- Helpers de bbox / geometría ----------

func dentroDeLima(lat, lon float64) bool {
	return lat >= minLatLima && lat <= maxLatLima &&
		lon >= minLonLima && lon <= maxLonLima
}

// Distancia Haversine en metros.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0 // radio de la Tierra en metros

	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dphi := (lat2 - lat1) * math.Pi / 180
	dlambda := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dphi/2)*math.Sin(dphi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dlambda/2)*math.Sin(dlambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// --------- Escritura de CSV de Lima ----------

func escribirCSVLima(outputDir string, nodos map[int64]NodoInfo, nodosLima map[int64]struct{},
	aristas []AristaCSV, reciclaje []RecyclingPoint) error {

	// Si no hay aristas ni reciclaje, no tiene sentido generar nada
	if len(aristas) == 0 && len(reciclaje) == 0 {
		log.Printf("Lima: sin datos (no se generan CSV)")
		return nil
	}

	// NODOS
	nodesPath := filepath.Join(outputDir, "lima_nodes.csv")
	nodesFile, err := os.Create(nodesPath)
	if err != nil {
		return fmt.Errorf("creando lima_nodes.csv: %w", err)
	}
	defer nodesFile.Close()

	nodesWriter := csv.NewWriter(nodesFile)
	defer nodesWriter.Flush()

	if err := nodesWriter.Write([]string{"id", "lat", "lon"}); err != nil {
		return fmt.Errorf("cabecera nodes: %w", err)
	}

	numNodos := 0
	for id := range nodosLima {
		info, ok := nodos[id]
		if !ok {
			continue
		}
		rec := []string{
			fmt.Sprintf("%d", id),
			fmt.Sprintf("%.7f", info.Lat),
			fmt.Sprintf("%.7f", info.Lon),
		}
		if err := nodesWriter.Write(rec); err != nil {
			return fmt.Errorf("escribiendo node: %w", err)
		}
		numNodos++
	}

	// ARISTAS
	edgesPath := filepath.Join(outputDir, "lima_edges.csv")
	edgesFile, err := os.Create(edgesPath)
	if err != nil {
		return fmt.Errorf("creando lima_edges.csv: %w", err)
	}
	defer edgesFile.Close()

	edgesWriter := csv.NewWriter(edgesFile)
	defer edgesWriter.Flush()

	if err := edgesWriter.Write([]string{"from", "to", "distance_m", "street_name"}); err != nil {
		return fmt.Errorf("cabecera edges: %w", err)
	}

	for _, e := range aristas {
		rec := []string{
			fmt.Sprintf("%d", e.From),
			fmt.Sprintf("%d", e.To),
			fmt.Sprintf("%.2f", e.Distance),
			e.Street,
		}
		if err := edgesWriter.Write(rec); err != nil {
			return fmt.Errorf("escribiendo edge: %w", err)
		}
	}

	// PUNTOS DE RECICLAJE
	if len(reciclaje) > 0 {
		recyPath := filepath.Join(outputDir, "lima_recycling.csv")
		recyFile, err := os.Create(recyPath)
		if err != nil {
			return fmt.Errorf("creando lima_recycling.csv: %w", err)
		}
		defer recyFile.Close()

		recyWriter := csv.NewWriter(recyFile)
		defer recyWriter.Flush()

		if err := recyWriter.Write([]string{"id", "lat", "lon", "name"}); err != nil {
			return fmt.Errorf("cabecera reciclaje: %w", err)
		}

		for _, rp := range reciclaje {
			rec := []string{
				fmt.Sprintf("%d", rp.ID),
				fmt.Sprintf("%.7f", rp.Lat),
				fmt.Sprintf("%.7f", rp.Lon),
				rp.Name,
			}
			if err := recyWriter.Write(rec); err != nil {
				return fmt.Errorf("escribiendo reciclaje: %w", err)
			}
		}
	}

	log.Printf("Lima: nodos=%d, aristas=%d, reciclaje=%d",
		numNodos, len(aristas), len(reciclaje))

	return nil
}
