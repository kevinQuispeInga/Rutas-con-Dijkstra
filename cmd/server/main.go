package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"

	"proyectoada/interno/graph"
	"proyectoada/interno/rutas"
)

// Punto de reciclaje cargado desde lima_recycling.csv
type RecyclingNode struct {
	ID   graph.NodeID `json:"id"`
	Lat  float64      `json:"lat"`
	Lon  float64      `json:"lon"`
	Name string       `json:"name"`

	// Nodo de calle más cercano (donde realmente termina la ruta)
	StreetNode graph.NodeID `json:"-"`
}

// respuesta de /api/puntos_reciclaje
type RecyclingResponse struct {
	ID         graph.NodeID `json:"id"`
	Name       string       `json:"name"`
	Lat        float64      `json:"lat"`
	Lon        float64      `json:"lon"`
	DistanceKm float64      `json:"distance_km"`
}

// respuesta de /api/ruta_reciclaje
type RouteResponse struct {
	DistanceM  float64       `json:"distance_m"`
	DistanceKm float64       `json:"distance_km"`
	Coords     [][2]float64  `json:"coords"` // [lat, lon]
	Streets    []string      `json:"streets"`
	Recycling  RecyclingNode `json:"recycling"`
}

// variables globales del servidor
var (
	grafo      *graph.Graph
	reciclajes []RecyclingNode
)

func main() {
	var err error

	// 1. Cargar grafo de Lima
	grafo, err = graph.LoadGraphFromCSV("./datos/lima_nodes.csv", "./datos/lima_edges.csv")
	if err != nil {
		log.Fatalf("error cargando grafo de Lima: %v", err)
	}
	log.Printf("Grafo Lima cargado: nodos=%d", len(grafo.Nodes))

	// 2. Cargar puntos de reciclaje
	reciclajes, err = loadRecycling("./datos/lima_recycling.csv")
	if err != nil {
		log.Fatalf("error cargando puntos de reciclaje: %v", err)
	}
	log.Printf("Puntos de reciclaje en Lima: %d", len(reciclajes))

	// 3. Conectar reciclajes y calcular StreetNode
	added := connectRecyclingToGraph(grafo, reciclajes)
	log.Printf("Se conectaron %d puntos de reciclaje al grafo (creando aristas de acceso)", added)

	// 4. Rutas HTTP
	http.HandleFunc("/api/puntos_reciclaje", handlePuntosReciclaje)
	http.HandleFunc("/api/ruta_reciclaje", handleRutaReciclaje)

	// 5. Servir archivos estáticos (interfaz web) desde ./web
	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/", fs)

	log.Println("Servidor escuchando en :8080 ...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// ----------------- handlers HTTP -----------------

func handlePuntosReciclaje(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")

	if latStr == "" || lonStr == "" {
		http.Error(w, "lat y lon son requeridos", http.StatusBadRequest)
		return
	}

	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	if err1 != nil || err2 != nil {
		http.Error(w, "lat/lon inválidos", http.StatusBadRequest)
		return
	}

	type cand struct {
		resp RecyclingResponse
		d2   float64
	}
	var cands []cand

	for _, rp := range reciclajes {
		d := haversine(lat, lon, rp.Lat, rp.Lon)
		cands = append(cands, cand{
			resp: RecyclingResponse{
				ID:         rp.ID,
				Name:       rp.Name,
				Lat:        rp.Lat,
				Lon:        rp.Lon,
				DistanceKm: d / 1000.0,
			},
			d2: d,
		})
	}

	// ordenar por distancia
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].d2 < cands[j].d2
	})

	const maxResults = 50
	var out []RecyclingResponse
	for i, c := range cands {
		if i >= maxResults {
			break
		}
		out = append(out, c.resp)
	}

	writeJSON(w, out)
}

func handleRutaReciclaje(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	idStr := r.URL.Query().Get("recycling_id")

	if latStr == "" || lonStr == "" || idStr == "" {
		http.Error(w, "lat, lon y recycling_id son requeridos", http.StatusBadRequest)
		return
	}

	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	idInt, err3 := strconv.ParseInt(idStr, 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		http.Error(w, "parámetros inválidos", http.StatusBadRequest)
		return
	}
	targetID := graph.NodeID(idInt)

	// 1. nodo de calle más cercano al usuario
	start := nearestNode(grafo, lat, lon)
	startNode := grafo.Nodes[start]
	if startNode == nil {
		http.Error(w, "no se encontró nodo de inicio", http.StatusInternalServerError)
		return
	}

	// 2. buscar info del reciclaje seleccionado
	var rp *RecyclingNode
	for i := range reciclajes {
		if reciclajes[i].ID == targetID {
			rp = &reciclajes[i]
			break
		}
	}
	if rp == nil {
		http.Error(w, "recycling_id no encontrado", http.StatusBadRequest)
		return
	}

	// 3. ruta más corta desde el usuario hasta el StreetNode del reciclaje
	if rp.StreetNode == 0 {
		http.Error(w, "punto de reciclaje sin StreetNode asignado", http.StatusInternalServerError)
		return
	}

	route, ok := rutas.ShortestPath(grafo, start, rp.StreetNode)
	if !ok || len(route.Nodes) == 0 {
		http.Error(w, "no se pudo trazar ruta hasta ese punto de reciclaje", http.StatusBadRequest)
		return
	}

	// 4. construir coords
	var coords [][2]float64
	for _, nid := range route.Nodes {
		n := grafo.Nodes[nid]
		if n == nil {
			continue
		}
		coords = append(coords, [2]float64{n.Lat, n.Lon})
	}

	// 5. calles
	streets := rutas.StreetsForRoute(grafo, route)

	resp := RouteResponse{
		DistanceM:  route.Distance,
		DistanceKm: route.Distance / 1000.0,
		Coords:     coords,
		Streets:    streets,
		Recycling:  *rp,
	}

	writeJSON(w, resp)
}

// ----------------- helpers compartidos -----------------

// carga lima_recycling.csv
func loadRecycling(path string) ([]RecyclingNode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)

	// saltar cabecera
	if _, err := r.Read(); err != nil {
		return nil, err
	}

	var res []RecyclingNode
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(rec) < 4 {
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
		name := rec[3]

		res = append(res, RecyclingNode{
			ID:   graph.NodeID(idInt),
			Lat:  lat,
			Lon:  lon,
			Name: name,
		})
	}

	return res, nil
}

// nodo del grafo más cercano a una lat/lon
func nearestNode(g *graph.Graph, lat, lon float64) graph.NodeID {
	var bestID graph.NodeID
	bestDist2 := 0.0
	first := true

	for id, n := range g.Nodes {
		dLat := n.Lat - lat
		dLon := n.Lon - lon
		d2 := dLat*dLat + dLon*dLon
		if first || d2 < bestDist2 {
			first = false
			bestDist2 = d2
			bestID = id
		}
	}
	return bestID
}

// Conecta puntos de reciclaje que no estén conectados al grafo,
// y además asigna el StreetNode (nodo de calle más cercano) a cada uno.
func connectRecyclingToGraph(g *graph.Graph, list []RecyclingNode) int {
	added := 0

	for i := range list {
		rp := &list[i]

		n := g.Nodes[rp.ID]

		// Caso 1: el nodo del reciclaje existe y tiene aristas -> usamos ese como StreetNode
		if n != nil && len(g.Adj[rp.ID]) > 0 {
			rp.StreetNode = rp.ID
			continue
		}

		// Caso 2: no existe o está aislado -> buscamos nodo de calle más cercano
		nearestID := nearestNode(g, rp.Lat, rp.Lon)
		rp.StreetNode = nearestID

		// Aseguramos que el nodo del reciclaje exista
		if n == nil {
			g.AddNode(rp.ID, rp.Lat, rp.Lon)
		}

		// Conectamos reciclaje <-> calle
		nearestNode := g.Nodes[nearestID]
		if nearestNode == nil {
			continue
		}

		dist := haversine(rp.Lat, rp.Lon, nearestNode.Lat, nearestNode.Lon)

		g.AddEdge(rp.ID, nearestID, dist, "Acceso al punto de reciclaje")
		g.AddEdge(nearestID, rp.ID, dist, "Acceso al punto de reciclaje")

		added++
	}

	return added
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, fmt.Sprintf("error codificando JSON: %v", err), http.StatusInternalServerError)
	}
}
