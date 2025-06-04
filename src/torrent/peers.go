package torrent

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

/**
The user is also a peer. This is his ID

It gets generated only once
*/
var clientPeerID string

func genClientPeerID() {
	prefix := []byte("-TM0001-")

	randSlice := make([]byte, 12)
	_, _ = rand.Read(randSlice)

	clientPeerID = string(append(prefix, randSlice...))
}

func getClientPeerID() string {
	if len(clientPeerID) == 0 {
		genClientPeerID()
		return clientPeerID
	}

	return clientPeerID
}

type peer struct {
	IP   net.IP
	Port uint16
}

func PrintPeersJson(peers []peer) {
	for _, peer := range peers {
		j, _ := json.MarshalIndent(&peer, "", "\t")
		fmt.Println(string(j))
	}
}

/**
The peers are defined by 6-byte strings, where the first 4 define the IP and the last 2 the port.
Both using network byte order (big-endian)
*/
func peersFromTrackerResponse(t *trackerResponse) ([]peer, error) {
	if t.Peers == "" {
		return nil, errors.New("tracker response doesn't contain peers")
	}

	const chunkSize = 6 // 6 bytes per peer

	if len(t.Peers) % chunkSize != 0 {
		return nil, errors.New("received malformed peers")
	}

	totalPeers := len(t.Peers) / 6

	peers := make([]peer, totalPeers)
	for i := range chunkSize {
		offset := i*chunkSize
		ipSlice := []byte(t.Peers)[offset:offset+4]
		portSlice := []byte(t.Peers)[offset+4:offset+6]

		newPeer := peer{
			IP: net.IP(ipSlice),
			Port: binary.BigEndian.Uint16(portSlice),
		}
		
		peers = append(peers, newPeer)
	}

	return peers, nil
}

