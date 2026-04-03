package rutas

import (
	"container/heap"
	"math"

	"proyectoada/interno/graph"
)

type Route struct {
	Nodes    []graph.NodeID
	Distance float64
}

// calcula la distancia total de una secuencia de nodos
func routeDistance(g *graph.Graph, nodes []graph.NodeID) float64 {
	if len(nodes) < 2 {
		return 0
	}
	total := 0.0
	for i := 0; i < len(nodes)-1; i++ {
		u := nodes[i]
		v := nodes[i+1]
		// buscamos la arista u -> v
		found := false
		for _, e := range g.Adj[u] {
			if e.To == v {
				total += e.Distance
				found = true
				break
			}
		}
		if !found {
			// si no encontramos, lo dejamos como está (ruta incompleta)
			// podrías sumar una penalización si quieres
		}
	}
	return total
}

// ---- implementación de Dijkstra ----

type pqItem struct {
	node graph.NodeID
	dist float64
	idx  int
}

type priorityQueue []*pqItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].dist < pq[j].dist }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].idx = i
	pq[j].idx = j
}
func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*pqItem)
	item.idx = n
	*pq = append(*pq, item)
}
func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// ShortestPath: ruta más corta entre src y dst
func ShortestPath(g *graph.Graph, src, dst graph.NodeID) (Route, bool) {
	dist := make(map[graph.NodeID]float64)
	prev := make(map[graph.NodeID]graph.NodeID)

	for id := range g.Nodes {
		dist[id] = math.Inf(1)
	}
	dist[src] = 0

	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &pqItem{node: src, dist: 0})

	visited := make(map[graph.NodeID]bool)

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*pqItem)
		u := item.node
		if visited[u] {
			continue
		}
		visited[u] = true

		if u == dst {
			break
		}

		for _, e := range g.Adj[u] {
			v := e.To
			if visited[v] {
				continue
			}
			alt := dist[u] + e.Distance
			if alt < dist[v] {
				dist[v] = alt
				prev[v] = u
				heap.Push(pq, &pqItem{node: v, dist: alt})
			}
		}
	}

	if src != dst {
		if _, ok := prev[dst]; !ok {
			return Route{}, false
		}
	}

	// reconstruimos ruta
	var nodes []graph.NodeID
	for v := dst; ; {
		nodes = append(nodes, v)
		if v == src {
			break
		}
		p, ok := prev[v]
		if !ok {
			break
		}
		v = p
	}
	// invertimos
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}

	return Route{
		Nodes:    nodes,
		Distance: routeDistance(g, nodes),
	}, true
}
