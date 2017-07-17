package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var allNodes []*Node
var allNeighbours = make(map[NeighbourId]*Neighbour)
var neighbUpdate = false
var nodeUpdate = false

var bigLock = &sync.RWMutex{}

type NodeId uint32
type Hash uint64

type Node struct {
	Types string
	Id    NodeId
	Hash  Hash
	Peer  []Peer
}

type Peer struct {
	PeerId NodeId
	Peid   uint32
	Leid   uint32
}

type NeighbourId struct {
	Types string
	Ip    string
	Zone  string
	Id    NodeId
	Eid   uint32
}

type Neighbour struct {
	NeighId     *NeighbourId
	NetworkHash Hash
	AllNodes    (map[NodeId]*Node)
	TimeStamp   time.Time
}

func CheckError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(0)
	}
}

func addNode(n *NeighbourId, id NodeId, hash Hash) {
	bigLock.Lock()
	if len(allNeighbours[*n].AllNodes) == 0 {
		allNeighbours[*n].AllNodes = make(map[NodeId]*Node)
	}
	node := Node{"node", id, hash, nil}
	allNeighbours[*n].AllNodes[id] = &node
	bigLock.Unlock()

}

func addPeer(n *NeighbourId, id NodeId, peerId NodeId, peid uint32, leid uint32) {
	bigLock.Lock()
	node, ok := allNeighbours[*n].AllNodes[id]
	if ok {
		peer := Peer{peerId, peid, leid}
		node.Peer = append(node.Peer, peer)
	}
	bigLock.Unlock()
}

func updatePeer(n *NeighbourId, id NodeId, peerId NodeId, peid uint32, leid uint32) {
	bigLock.RLock()
	exist := false
	tmp := allNeighbours[*n].AllNodes[id]
	for _, value := range tmp.Peer {
		if value.PeerId == peerId && value.Peid == peid && value.Leid == leid {
			fmt.Println("Peer exist")
			exist = true
		}
	}
	bigLock.RUnlock()
	if !exist {
		addPeer(n, id, peerId, peid, leid)
		fmt.Println("Nouveau Peer")
	}
}

func updateNeighbour(n *Neighbour, h Hash) int {
	bigLock.Lock()
	for i, value := range allNeighbours {
		if value.NeighId.Ip == n.NeighId.Ip && value.NeighId.Zone == n.NeighId.Zone &&
			value.NeighId.Ip == n.NeighId.Ip && value.NeighId.Eid == n.NeighId.Eid &&
			value.NetworkHash != h {

			allNeighbours[i].NetworkHash = h
			allNeighbours[i].TimeStamp = time.Now()
			return 2
		} else if value.NeighId.Ip == n.NeighId.Ip && value.NeighId.Zone == n.NeighId.Zone &&
			value.NeighId.Ip == n.NeighId.Ip && value.NeighId.Eid == n.NeighId.Eid {

			allNeighbours[i].TimeStamp = time.Now()
			return 3
		}
	}
	allNeighbours[*n.NeighId] = n
	return 1
}

func skip(n int, r io.Reader) {
	var dummy uint8
	for i := 0; i < n; i++ {
		binary.Read(r, binary.BigEndian, &dummy)
	}
}

func readPacket(Conn *net.UDPConn) {
	buffer := make([]byte, 64*1024)

	for {
		n, addr, err := Conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error ReadFromUDP: ", err)
			continue
		}
		var reader io.Reader = bytes.NewBuffer(buffer[0:n])
		var nodeId NodeId
		var endpointId uint32
		i := 0
		for i < n {
			var t, l uint16
			binary.Read(reader, binary.BigEndian, &t)
			binary.Read(reader, binary.BigEndian, &l)
			i += 4

			switch t {

			/* Node endpoint */
			case 3:
				var nid NodeId
				var eid uint32
				binary.Read(reader, binary.BigEndian, &nid)
				binary.Read(reader, binary.BigEndian, &eid)
				nodeId, endpointId = nid, eid
				fmt.Printf("Received: NODE-ENPOINT (%d) %x %x\n", t, nid, eid)

			/* Network state */
			case 4:
				var hash Hash
				binary.Read(reader, binary.BigEndian, &hash)
				fmt.Printf("Received: NETWORK-STATE (%d) %x\n ", t, hash)

				neighId := NeighbourId{"neighbour", addr.IP.String(), addr.Zone, nodeId, endpointId}
				neighbour := Neighbour{&neighId, hash, nil, time.Now()}

				ok := updateNeighbour(&neighbour, hash)
				bigLock.Unlock()

				if ok == 1 || ok == 2 {

					neighbUpdate = true

					// SEND REQUEST-NETWORK-STATE
					buff := new(bytes.Buffer)

					var data = []interface{}{
						uint16(1),
						uint16(0),
					}

					for _, b := range data {
						binary.Write(buff, binary.BigEndian, b)
					}

					_, err := Conn.WriteToUDP(buff.Bytes(), addr)
					if err != nil {
						fmt.Println("Error WriteToUDP: ", err)
					}

					fmt.Println("-> REQUEST NETWORK-STATE")
				}

			/* Node-State */
			case 5:
				fmt.Printf("Received: NODE-STATE (%d) ", t)
				fmt.Println("taille: ", l)

				var id NodeId
				var v2, v3 uint32 // v2: seqno v3: time since origination
				var hash Hash     // Node hash
				binary.Read(reader, binary.BigEndian, &id)
				binary.Read(reader, binary.BigEndian, &v2)
				binary.Read(reader, binary.BigEndian, &v3)
				binary.Read(reader, binary.BigEndian, &hash)
				fmt.Printf("Node id: %x seqno: %x time: %x hash: %x \n", id, v2, v3, hash)

				neighId := NeighbourId{"neighbour", addr.IP.String(), addr.Zone, nodeId, endpointId}
				_, ok := allNeighbours[neighId].AllNodes[id]

				if !ok {
					addNode(&neighId, id, hash)
				}
				if allNeighbours[neighId].AllNodes[id].Hash != hash || !ok {
					allNeighbours[neighId].AllNodes[id].Hash = hash

					nodeUpdate = true

					allNeighbours[neighId].AllNodes[id].Peer = nil

					// REQUEST NODE-STATE
					buff := new(bytes.Buffer)
					var data = []interface{}{
						uint16(2),
						uint16(4),
						id,
					}
					for _, b := range data {
						binary.Write(buff, binary.BigEndian, b)
					}
					_, err = Conn.WriteToUDP(buff.Bytes(), addr)
					if err != nil {
						fmt.Println("Error WriteToUDP: ", err)
					}
					fmt.Printf("-> REQUEST NODE-STATE %x \n", id)
				}
				/* Node State longs */
				if l > 20 {
					j := 0
					for j < int(l) {
						var t8, l8 uint16
						var peerId NodeId
						var peid, leid uint32
						binary.Read(reader, binary.BigEndian, &t8)
						binary.Read(reader, binary.BigEndian, &l8)
						j += 4
						fmt.Println("type ", t8, "taille ", l8)
						/* 8 : Type Peer */
						if t8 == 8 {
							binary.Read(reader, binary.BigEndian, &peerId)
							binary.Read(reader, binary.BigEndian, &peid)
							binary.Read(reader, binary.BigEndian, &leid)

							updatePeer(&neighId, id, peerId, peid, leid)

						} else {
							skip(int(l8), reader)
						}
						skip((-int(l8))&3, reader)
						j += int(l8) + ((-int(l8)) & 3)
					}
				}
			default:
				fmt.Println("type ", t)
			}

			i += int(l)

		}

		if err != nil {
			fmt.Println("Error: ", err)
		}
	}
}

func peerSetup(ifi *net.Interface) {
	ServerAddr, err := net.ResolveUDPAddr("udp6", "[ff02::11]:8231")
	CheckError(err)

	Conn, err := net.ListenMulticastUDP("udp6", ifi, ServerAddr)
	if err != nil {
		fmt.Println("Error: ", err)
	} else {
		readPacket(Conn)
	}
}

func checkNeighbours() {
	for {
		bigLock.Lock()
		for _, value := range allNeighbours {
			if (time.Since(value.TimeStamp)).Seconds() > 105 {
				fmt.Println("Neighbour died: ", (time.Since(value.TimeStamp)).Seconds())
				delete(allNeighbours, *value.NeighId)
				for _, val := range allNeighbours {
					delete(allNeighbours, *value.NeighId)
					delete(val.AllNodes, value.NeighId.Id)
				}
				nodeUpdate = true
			}
		}
		bigLock.Unlock()
		time.Sleep(30 * time.Second)
	}
}

func handler(ws *websocket.Conn) {

	if len(allNeighbours) > 0 {
		var neighbours []*NeighbourId
		for _, value := range allNeighbours {
			neighbours = append(neighbours, value.NeighId)
		}
		if err := websocket.JSON.Send(ws, neighbours); err != nil {
			fmt.Println("Can't send")
		}
		if err := websocket.JSON.Send(ws, allNodes); err != nil {
			fmt.Println("Can't send")
		}
	}

	for {
		if len(allNeighbours) > 0 {

			if neighbUpdate {
				var neighbours []*NeighbourId
				for _, value := range allNeighbours {
					neighbours = append(neighbours, value.NeighId)
				}
				err := websocket.JSON.Send(ws, neighbours)
				if err != nil {
					fmt.Println("Can't send")
				} else {
					neighbUpdate = false
				}
			}

			if nodeUpdate {
				err := websocket.JSON.Send(ws, allNodes)
				if err != nil {
					fmt.Println("Can't send")
				} else {
					nodeUpdate = false
				}
			}

		}
		time.Sleep(5 * time.Second)
	}

}

func main() {
	ifiName := os.Args[1:]

	if len(ifiName) == 0 {
		go peerSetup(nil)
	} else {
		for _, name := range ifiName {
			ifi, err := net.InterfaceByName(name)
			CheckError(err)
			go peerSetup(ifi)
		}
	}

	http.Handle("/websocket", websocket.Handler(handler))
	http.Handle("/", http.FileServer(http.Dir("./static")))
	go http.ListenAndServe(":8000", nil)

	go checkNeighbours()

	for {
		if nodeUpdate == true {
			bigLock.Lock()

			var nodes = make(map[NodeId]*Node)
			for _, value := range allNeighbours {
				for _, val := range value.AllNodes {
					_, ok := nodes[val.Id]
					if !ok {
						nodes[val.Id] = val
					}
				}
			}

			i := 0
			allNodes = allNodes[:0]
			for _, value := range nodes {
				allNodes = append(allNodes, value)
				i++
			}
			bigLock.Unlock()
		}
		time.Sleep(7 * time.Second)
	}

}
