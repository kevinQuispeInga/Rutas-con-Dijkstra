package graph

type NodeID int64

type Node struct {
	ID  NodeID
	Lat float64
	Lon float64
}

type Edge struct {
	From     NodeID
	To       NodeID
	Distance float64
	Street   string
}

type Graph struct {
	Nodes map[NodeID]*Node
	Adj   map[NodeID][]Edge
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[NodeID]*Node),
		Adj:   make(map[NodeID][]Edge),
	}
}

func (g *Graph) AddNode(id NodeID, lat, lon float64) {
	if _, exists := g.Nodes[id]; exists {
		return
	}
	g.Nodes[id] = &Node{
		ID:  id,
		Lat: lat,
		Lon: lon,
	}
}

func (g *Graph) AddEdge(from, to NodeID, dist float64, street string) {
	g.Adj[from] = append(g.Adj[from], Edge{
		From:     from,
		To:       to,
		Distance: dist,
		Street:   street,
	})
}
