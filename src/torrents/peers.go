package torrents

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TatuMon/bittorrent-client/logger"
	"github.com/sirupsen/logrus"
)

const handshakeLen = 68
const maxReqBacklog = 5

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
	peer       Peer
	conn       net.Conn
	unchoked   bool
	interested bool
	reqBacklog int
	bitfield   *Bitfield
}

func (p *PeerConn) read() (*Message, error) {
	msg, err := MessageFromStream(p.conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read from connection: %w", err)
	}

	if msg == nil {
		logger.LogRecvMessage("received message of type 'keep alive' from %s", p.peer.String())
		return nil, nil
	} else {
		logger.LogRecvMessage("received message of type '%s' from %s", msg.ID.String(), p.peer.String())
	}

	switch msg.ID {
	case MsgChoke:
		p.unchoked = false
	case MsgUnchoke:
		p.unchoked = true
	case MsgBitField:
		p.bitfield = (*Bitfield)(&msg.Payload)
	case MsgHave:
		if p.bitfield != nil {
			p.bitfield.SetPiece(int(binary.BigEndian.Uint32(msg.Payload)))
		}
	// I dont expect to receive other type of messages
	}

	return msg, nil
}

func (p *PeerConn) sendInterestedMsg() error {
	msg := Message{
		ID: MsgInterested,
	}

	m := msg.Serialize()
	if _, err := p.conn.Write(m); err != nil {
		return fmt.Errorf("failed to write to connection: %w", err)
	}

	logger.LogSentMessage("'%s' message sent to peer %s", msg.ID.String(), p.peer.String())

	return nil
}

func (p *PeerConn) sendUnchoke() error {
	msg := Message{
		ID: MsgUnchoke,
	}

	m := msg.Serialize()
	if _, err := p.conn.Write(m); err != nil {
		return fmt.Errorf("failed to write to connection: %w", err)
	}

	logger.LogSentMessage("'%s' message sent to peer %s", msg.ID.String(), p.peer.String())

	return nil
}

func (p *PeerConn) sendRequestMsg(pieceIndex uint32, beginOffset uint32, blockLen uint32) error {
	payloadBuf := make([]byte, 12)
	binary.BigEndian.PutUint32(payloadBuf[0:4], pieceIndex)
	binary.BigEndian.PutUint32(payloadBuf[4:8], beginOffset)
	binary.BigEndian.PutUint32(payloadBuf[8:12], blockLen)

	msg := Message{
		ID: MsgRequest,
		Payload: payloadBuf,
	}

	m := msg.Serialize()
	if _, err := p.conn.Write(m); err != nil {
		return fmt.Errorf("failed to write to connection: %w", err)
	}

	logger.LogSentMessage("'%s' message sent to peer %s", msg.ID.String(), p.peer.String())

	return nil
}

func connectToPeer(torr *Torrent, peer Peer) (*PeerConn, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to make TCP connection: %w", err)
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetDeadline(time.Time{})

	handshake := HandshakeFromTorrent(torr)
	if _, err := conn.Write(handshake.Serialize()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failure at protocol handshake: %w", err)
	}

	res := make([]byte, handshakeLen)
	if _, err := conn.Read(res); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read peer's handshake response: %w", err)
	}
	handshakeRes, err := HandshakeFromStream(res)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failure at protocol handshake response: %w", err)
	}

	if handshake.InfoHash != handshakeRes.InfoHash {
		conn.Close()
		return nil, errors.New("handshake failure: info hashes dont match")
	}

	pc := &PeerConn{
		peer:       peer,
		conn:       conn,
		unchoked:   false,
		interested: false,
	}

	for !pc.unchoked {
		_, err := pc.read()
		if err != nil {
			return nil, fmt.Errorf("failed to wait for bitfield: %w", err)
		}
	}

	return pc, nil
}

/*
If workCtx is done, the channel is not yet closed, but no more peers are added to it from this function.
*/
func connectPeersAsync(torr *Torrent, peers []Peer, workCtx context.Context) chan *PeerConn {
	channel := make(chan *PeerConn, len(peers))
	peersConnectedTotal := atomic.Uint64{}
	connsAttempts := atomic.Uint64{}
	totalPeers := len(peers)

	peersConnsWorkCtx, peersConnsWorkCtxCancel := context.WithCancel(workCtx)

	// Channel cleanup
	go func() {
		closing := sync.OnceFunc(func() { close(channel) })

		// "channel" should be closed here when either workCtx is done or all peers are processed
		select {
		case <-workCtx.Done():
			peersConnsWorkCtxCancel()
			closing()
			return
		case <-peersConnsWorkCtx.Done():
			closing()
			logrus.Debug("connected to all available peers")
			return
		}
	}()

	for _, p := range peers {
		peer := p
		go func() {
			defer func() {
				connsAttempts.Add(1)
				if connsAttempts.Load() == uint64(totalPeers) {
					peersConnsWorkCtxCancel()
				}
			}()

			pConn, err := connectToPeer(torr, peer)
			if err != nil {
				logrus.Warnf("failed to connect to peer %s: %s", peer.String(), err.Error())
				return
			}

			select {
			case <-workCtx.Done():
				return
			case channel <- pConn:
				peersConnectedTotal.Add(1)
				logrus.Debugf("%d/%d peers connected", peersConnectedTotal.Load(), totalPeers)
			}
		}()
	}

	if totalPeers == 0 {
		peersConnsWorkCtxCancel()
	}

	return channel
}
