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

const handshakeLen = 68

/*
*
The user is also a peer. This is his ID

It gets generated only once when calling getClientPeerID
*/
var clientPeerID string

/*
*
This function MUST only be called by getClientPeerID
*/
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

/*
*
The peers are defined by 6-byte strings, where the first 4 define the IP and the last 2 the port.
Both using network byte order (big-endian)
*/
func peersFromTrackerResponse(t *trackerResponse) ([]Peer, error) {
	peersBin := []byte(t.Peers)

	if len(peersBin) == 0 {
		return nil, errors.New("tracker response doesn't contain peers")
	}

	const chunkSize = 6 // 6 bytes per peer
	totalPeers := len(peersBin) / 6
	if len(peersBin)%chunkSize != 0 {
		return nil, errors.New("received malformed peers")
	}

	peers := make([]Peer, totalPeers)
	for i := range totalPeers {
		offset := i * chunkSize
		peers[i].IP = peersBin[offset : offset+4]
		peers[i].Port = binary.BigEndian.Uint16(peersBin[offset+4 : offset+6])
	}

	return peers, nil
}

func PrintPeersJson(peers []Peer) {
	for _, peer := range peers {
		peer.PrintJson()
	}
}

type Handshake struct {
	Pstr     string
	InfoHash Sha1Checksum
	PeerID   Sha1Checksum
}

/*
*
https://wiki.theory.org/BitTorrentSpecification#Handshake
*/
func (h *Handshake) Serialize() []byte {
	var buf bytes.Buffer
	var reserved [8]byte

	buf.WriteByte(byte(len(h.Pstr)))
	buf.Write([]byte("BitTorrent protocol"))
	buf.Write(reserved[:])
	buf.Write(h.InfoHash[:])
	buf.Write(h.PeerID[:])

	return buf.Bytes()
}

func HandshakeFromTorrent(torr *Torrent) Handshake {
	return Handshake{
		Pstr:     "BitTorrent protocol",
		InfoHash: torr.InfoHash,
		PeerID:   Sha1Checksum([]byte(getClientPeerID())),
	}
}

func HandshakeFromStream(r []byte) (*Handshake, error) {
	buf := bytes.NewBuffer(r)

	if buf.Len() == 0 {
		return nil, errors.New("empty handshake response")
	}

	pstrlen, _ := buf.ReadByte()

	pstrbuf := make([]byte, int(pstrlen))
	if _, err := io.ReadFull(buf, pstrbuf); err != nil {
		return nil, fmt.Errorf("failed to get protocol string: %w", err)
	}

	buf.Next(8) // Discard the "reserved" part of the handshake

	infoHashBuf := make([]byte, 20)
	if _, err := io.ReadFull(buf, infoHashBuf); err != nil {
		return nil, fmt.Errorf("failed to get info hash: %w", err)
	}

	peerIDBuf := make([]byte, 20)
	if _, err := io.ReadFull(buf, peerIDBuf); err != nil {
		return nil, fmt.Errorf("failed to get peer ID: %w", err)
	}

	return &Handshake{
		Pstr:     string(pstrbuf),
		InfoHash: Sha1Checksum(infoHashBuf),
		PeerID:   Sha1Checksum(peerIDBuf),
	}, nil
}

type PeerConn struct {
	peer       *Peer
	conn       *net.Conn
	unchoked   bool
	interested bool
}

func connectToPeer(torr *Torrent, peer *Peer) (*PeerConn, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to make TCP connection: %w", err)
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetDeadline(time.Time{})

	handshake := HandshakeFromTorrent(torr)
	if _, err := conn.Write(handshake.Serialize()); err != nil {
		return nil, fmt.Errorf("failure at protocol handshake: %w", err)
	}

	res := make([]byte, handshakeLen)
	if _, err := conn.Read(res); err != nil {
		return nil, fmt.Errorf("failed to read peer's handshake response: %w", err)
	}
	handshakeRes, err := HandshakeFromStream(res)
	if err != nil {
		return nil, fmt.Errorf("failure at protocol handshake response: %w", err)
	}

	if handshake.InfoHash != handshakeRes.InfoHash {
		return nil, errors.New("handshake failure: info hashes dont match")
	}

	return &PeerConn{
		peer:     peer,
		conn:     &conn,
		unchoked: false,
		interested: false,
	}, nil
}

func connectPeersAsync(torr *Torrent, peers []Peer) chan *PeerConn {
	channel := make(chan *PeerConn, len(peers))

	for i, peer := range peers {
		go func() {
			pConn, err := connectToPeer(torr, &peer)
			if err != nil {
				logrus.Warnf("failed to connect to peer %s: %s", peer.String(), err.Error())
			}

			channel <- pConn

			if i == len(peers) - 1 {
				close(channel)
			}
		}()
	}

	return channel
}
