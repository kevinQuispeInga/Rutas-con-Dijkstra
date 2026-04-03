package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"

	"proyectoada/interno/graph"
	"proyectoada/interno/rutas"
)

// Punto de reciclaje cargado desde lima_recycling.csv
type RecyclingNode struct {
	ID   graph.NodeID
	Lat  float64
	Lon  float64
	Name string
}

func main() {
	// 1. Cargar grafo de Lima
	g, err := graph.LoadGraphFromCSV("./datos/lima_nodes.csv", "./datos/lima_edges.csv")
	if err != nil {
		log.Fatalf("error cargando grafo de Lima: %v", err)
	}
	log.Printf("Grafo Lima: nodos=%d", len(g.Nodes))

	// 2. Cargar puntos de reciclaje
	recy, err := loadRecycling("./datos/lima_recycling.csv")
	if err != nil {
		log.Fatalf("error cargando puntos de reciclaje: %v", err)
	}
	if len(recy) == 0 {
		log.Fatalf("no hay puntos de reciclaje en lima_recycling.csv")
	}
	log.Printf("Puntos de reciclaje en Lima: %d", len(recy))

	// 3. Conectar reciclajes que estén aislados al grafo
	added := connectRecyclingToGraph(g, recy)
	log.Printf("Se conectaron %d puntos de reciclaje al grafo (creando aristas de acceso)", added)

	// 4. Simular ubicación del usuario (lat, lon)
	//    Cambia estas coordenadas para probar otros puntos de la ciudad.
	userLat := recy[0].Lat
	userLon := recy[0].Lon
	log.Printf("Ubicación usuario: lat=%.6f, lon=%.6f", userLat, userLon)

	// 5. Nodo de calle más cercano al usuario
	start := nearestNode(g, userLat, userLon)
	startNode := g.Nodes[start]
	if startNode == nil {
		log.Fatalf("no se encontró nodo de inicio")
	}
	log.Printf("Nodo más cercano al usuario: ID=%d, lat=%.6f, lon=%.6f",
		start, startNode.Lat, startNode.Lon)

	// 6. Buscar el punto de reciclaje más cercano con ruta posible
	nearest, route, ok := findNearestRecyclingWithRoute(g, recy, start, userLat, userLon)
	if !ok {
		log.Fatalf("No se encontró ninguna ruta a ningún punto de reciclaje (componentes desconectados)")
	}

	log.Printf("Punto de reciclaje elegido: ID=%d, nombre=%q, lat=%.6f, lon=%.6f",
		nearest.ID, nearest.Name, nearest.Lat, nearest.Lon)

	log.Printf("Ruta encontrada: nodos=%d, distancia≈%.2f m (%.2f km)",
		len(route.Nodes), route.Distance, route.Distance/1000.0)

	// 7. Mostrar primeros 10 nodos de la ruta
	fmt.Println("Primeros 10 nodos de la ruta:")
	for i := 0; i < len(route.Nodes) && i < 10; i++ {
		nid := route.Nodes[i]
		n := g.Nodes[nid]
		if n == nil {
			continue
		}
		fmt.Printf("%2d: ID=%d, lat=%.6f, lon=%.6f\n", i, nid, n.Lat, n.Lon)
	}

	// 8. Mostrar algunas calles que recorre
	streets := rutas.StreetsForRoute(g, route)
	fmt.Println("\nCalles recorridas (primeras 20):")
	for i := 0; i < len(streets) && i < 20; i++ {
		fmt.Printf("- %s\n", streets[i])
	}
}

// ---------- helpers ----------

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

// Conecta puntos de reciclaje que no estén en el grafo o que no tengan aristas,
// añadiendo una arista "de acceso" al nodo de calle más cercano.
// Devuelve cuántos puntos fueron conectados.
func connectRecyclingToGraph(g *graph.Graph, list []RecyclingNode) int {
	added := 0

	for _, rp := range list {
		// ¿Existe como nodo?
		n := g.Nodes[rp.ID]

		// Si no existe, lo creamos como nodo aislado
		if n == nil {
			g.AddNode(rp.ID, rp.Lat, rp.Lon)
		}

		// ¿Ya tiene aristas? (grado > 0)
		if len(g.Adj[rp.ID]) > 0 {
			continue // ya está conectado a la red
		}

		// Buscar nodo de calle más cercano
		nearestID := nearestNode(g, rp.Lat, rp.Lon)
		nearestNode := g.Nodes[nearestID]
		if nearestNode == nil {
			continue
		}

		// Distancia entre el reciclaje y la calle más cercana
		dist := haversine(rp.Lat, rp.Lon, nearestNode.Lat, nearestNode.Lon)

		// Conectamos reciclaje <-> calle
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

// Busca el punto de reciclaje más cercano al usuario PARA EL QUE SÍ
// exista una ruta desde el nodo start. Devuelve el reciclaje y la ruta.
func findNearestRecyclingWithRoute(
	g *graph.Graph,
	list []RecyclingNode,
	start graph.NodeID,
	userLat, userLon float64,
) (RecyclingNode, rutas.Route, bool) {

	type cand struct {
		rp   RecyclingNode
		dist float64
	}
	var cands []cand

	// construir lista con distancia al usuario
	for _, rp := range list {
		dLat := rp.Lat - userLat
		dLon := rp.Lon - userLon
		d2 := dLat*dLat + dLon*dLon
		cands = append(cands, cand{
			rp:   rp,
			dist: d2,
		})
	}

	// ordenar por distancia creciente
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].dist < cands[j].dist
	})

	// probar rutas en ese orden
	for _, c := range cands {
		route, ok := rutas.ShortestPath(g, start, c.rp.ID)
		if ok {
			return c.rp, route, true
		}
	}

	return RecyclingNode{}, rutas.Route{}, false
}
