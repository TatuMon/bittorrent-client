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

	"github.com/sirupsen/logrus"
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
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

func (p *Peer) PrintJson() {
	j, _ := json.MarshalIndent(&p, "", "\t")
	fmt.Println(string(j))
}

func PrintPeersJson(peers []Peer) {
	for _, peer := range peers {
		peer.PrintJson()
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

func HandshakeFromStream(r io.Reader) (*Handshake, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, fmt.Errorf("failed to read handshake response: %w", err)
	}

	if buf.Len() == 0 {
		return nil, errors.New("empty handshake response")
	}

	pstrlen, _ := buf.ReadByte()

	pstrbuf := make([]byte, int(pstrlen))
	if _, err := io.ReadFull(&buf, pstrbuf); err != nil {
		return nil, fmt.Errorf("failed to get protocol string: %w", err)
	}

	buf.Next(8) // Discard the "reserved" part of the handshake

	infoHashBuf := make([]byte, 20)
	if _, err := io.ReadFull(&buf, infoHashBuf); err != nil {
		return nil, fmt.Errorf("failed to get info hash: %w", err)
	}

	peerIDBuf := make([]byte, 20)
	if _, err := io.ReadFull(&buf, peerIDBuf); err != nil {
		return nil, fmt.Errorf("failed to get peer ID: %w", err)
	}
	
	return &Handshake{
		Pstr: string(pstrbuf),
		InfoHash: Sha1Checksum(infoHashBuf),
		PeerID: Sha1Checksum(peerIDBuf),
	}, nil
}

/**
https://wiki.theory.org/BitTorrentSpecification#Handshake
*/
func (h *Handshake) Serialize() []byte {
	var buf bytes.Buffer
	
	buf.WriteByte(byte(len(h.Pstr)))

	var reserved [8]byte
	buf.Write(reserved[:])
	buf.Write(h.InfoHash[:])
	buf.Write(h.PeerID[:])

	return buf.Bytes()
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
	
		if !newPeer.IP.Equal(net.IPv4zero) && newPeer.Port != 0 {
			peers = append(peers, newPeer)
		}
	}

	return peers, nil
}

func ConnectToPeer(torr *Torrent, peer Peer) error {
	conn, err := net.DialTimeout("tcp", peer.String(), 3 * time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", peer.String(), err)
	}
	defer conn.Close()

	handshake := HandshakeFromTorrent(torr)
	if _, err := conn.Write(handshake.Serialize()); err != nil {
		return fmt.Errorf("failure at protocol handshake: %w", err)
	}

	handshakeRes, err := HandshakeFromStream(conn)
	if err != nil {
		return fmt.Errorf("failure at protocol handshake response: %w", err)
	}

	if handshake.InfoHash != handshakeRes.InfoHash {
		return errors.New("handshake failure: info hashes dont match")
	}

	logrus.Debugf("successfuly connected to peer %s", peer.String())
	fmt.Printf("hola")

	return nil
}
