package torrents

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
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

type Peer struct {
	IP   net.IP
	Port uint16
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s:%s", p.IP, p.Port)
}

func PrintPeersJson(peers []Peer) {
	for _, peer := range peers {
		j, _ := json.MarshalIndent(&peer, "", "\t")
		fmt.Println(string(j))
	}
}

type Handshake struct {
	Pstr string
	InfoHash Sha1Checksum
	PeerID Sha1Checksum
}

func HandshakeFromTorrent(torr *Torrent) Handshake {
	return Handshake{
		Pstr: "BitTorrent protocol",
		InfoHash: torr.InfoHash,
		PeerID: Sha1Checksum([]byte(getClientPeerID())),
	}
}

/**
TODO
Finish this
*/
func HandshakeFromStream(r io.Reader) (*Handshake, error) {
	

	r.Read()
}

/**
https://wiki.theory.org/BitTorrentSpecification#Handshake
*/
func (h *Handshake) Serialize() []byte {
	buf := make([]byte, 68)

	// 68 bytes in total
	pstrlen := byte(len(h.Pstr)) // 1 byte
	var reserved [8]byte // 8 bytes

	buf[0] = pstrlen
	writeIndex := 1
	writeIndex += copy(buf[writeIndex:], []byte(h.Pstr))
	writeIndex += copy(buf[writeIndex:], reserved[:])
	writeIndex += copy(buf[writeIndex:], h.InfoHash[:])
	writeIndex += copy(buf[writeIndex:], h.PeerID[:])

	return buf
}

/**
The peers are defined by 6-byte strings, where the first 4 define the IP and the last 2 the port.
Both using network byte order (big-endian)
*/
func peersFromTrackerResponse(t *trackerResponse) ([]Peer, error) {
	if t.Peers == "" {
		return nil, errors.New("tracker response doesn't contain peers")
	}

	const chunkSize = 6 // 6 bytes per peer

	if len(t.Peers) % chunkSize != 0 {
		return nil, errors.New("received malformed peers")
	}

	totalPeers := len(t.Peers) / 6

	peers := make([]Peer, totalPeers)
	for i := range chunkSize {
		offset := i*chunkSize
		ipSlice := []byte(t.Peers)[offset:offset+4]
		portSlice := []byte(t.Peers)[offset+4:offset+6]

		newPeer := Peer{
			IP: net.IP(ipSlice),
			Port: binary.BigEndian.Uint16(portSlice),
		}
		
		peers = append(peers, newPeer)
	}

	return peers, nil
}

func connectToPeer(torr *Torrent, peer Peer) error {
	conn, err := net.DialTimeout("tcp", peer.String(), 3 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", peer.String(), err)
	}

	handshake := buildHandshake(torr)
	if _, err := conn.Write(handshake); err != nil {
		return fmt.Errorf("failure at protocol handshake: %w", err)
	}

	return nil
}
