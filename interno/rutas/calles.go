package rutas

import "proyectoada/interno/graph"

// StreetsForRoute devuelve los nombres de calles recorridas por una ruta,
// evitando repetir nombres consecutivos.
func StreetsForRoute(g *graph.Graph, r Route) []string {
	var streets []string
	if len(r.Nodes) < 2 {
		return streets
	}

	var lastStreet string

	for i := 0; i < len(r.Nodes)-1; i++ {
		u := r.Nodes[i]
		v := r.Nodes[i+1]

		var street string
		for _, e := range g.Adj[u] {
			if e.To == v {
				street = e.Street
				break
			}
		}
		if street == "" {
			continue
		}
		if street == lastStreet {
			continue
		}
		streets = append(streets, street)
		lastStreet = street
	}

	return streets
}
