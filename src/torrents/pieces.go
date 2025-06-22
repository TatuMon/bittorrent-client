package torrents

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// 16KB maximum for compatibility: https://wiki.theory.org/BitTorrentSpecification#request:_.3Clen.3D0013.3E.3Cid.3D6.3E.3Cindex.3E.3Cbegin.3E.3Clength.3E
const blockSize = 16 * 1024
const maxPipelinedRequests = 5

type PieceBlock struct {
	Index uint32
	Begin uint32
	Data  []byte
}

func PieceBlockFromMessage(msg *Message) (*PieceBlock, error) {
	if msg.ID != MsgPiece {
		return nil, fmt.Errorf("wrong message given: must be of type 'piece' but '%s' given", msg.ID.String())
	}

	p := PieceBlock{
		Index: binary.BigEndian.Uint32(msg.Payload[0:4]),
		Begin: binary.BigEndian.Uint32(msg.Payload[4:8]),
		Data:  msg.Payload[8:],
	}

	return &p, nil
}

/*
PieceProgress represents the download process of a single piece.

A single PieceProgress MUST be handled by at most ONE worker goroutine
*/
type PieceProgress struct {
	index        int
	size         uint
	buf          []byte
	expectedHash Sha1Checksum
	completed    bool
	requested    uint
	downloaded   uint
}

func newPieceProgress(index int, expectedHash Sha1Checksum, pieceSize uint) *PieceProgress {
	return &PieceProgress{
		index:        index,
		size:         pieceSize,
		buf:          make([]byte, pieceSize),
		expectedHash: expectedHash,
		completed:    false,
	}
}

func (p *PieceProgress) calcNextBlockSize() uint {
	// Last block might be smaller than the rest
	if p.size-p.requested < uint(blockSize) {
		s := p.size - p.requested
		return s
	}

	return uint(blockSize)
}

func (p *PieceProgress) ValidateHash() error {
	h := sha1.Sum(p.buf)

	if !bytes.Equal(h[:], p.expectedHash[:]) {
		return fmt.Errorf("hash mismatch.")
	}

	return nil
}

func genPiecesProgresses(torr *Torrent) chan *PieceProgress {
	channel := make(chan *PieceProgress, len(torr.PiecesHashes))

	for i, hash := range torr.PiecesHashes {
		pp := newPieceProgress(i, hash, torr.calculatePieceSize(uint(i)))
		channel <- pp
	}

	return channel
}

func attemptPieceDownload(peer *PeerConn, piece *PieceProgress) error {
	peer.conn.SetDeadline(time.Now().Add(time.Second * 30))
	defer peer.conn.SetDeadline(time.Time{})

	for piece.downloaded < piece.size {
		if peer.unchoked {
			for peer.reqBacklog < maxReqBacklog && piece.requested < piece.size {
				blockSize := piece.calcNextBlockSize()
				err := peer.sendRequestMsg(uint32(piece.index), uint32(piece.requested), uint32(blockSize))
				if err != nil {
					return fmt.Errorf("failed to request piece %d: %w", piece.index, err)
				}

				peer.reqBacklog++
				piece.requested += blockSize
			}
		}

		msg, err := peer.read()
		if err != nil {
			return fmt.Errorf("failed to read from peer: %w", err)
		}

		if msg.ID == MsgPiece {
			block, err := PieceBlockFromMessage(msg)

			if err != nil {
				return fmt.Errorf("failed to get block from message: %w", err)
			}

			if block.Index != uint32(piece.index) {
				return fmt.Errorf("wrong piece block: expected index %d, got %d", piece.index, block.Index)
			}

			if block.Begin >= uint32(piece.size) {
				return fmt.Errorf("wrong piece block: offset exceedes piece size. %d >= %d", block.Begin, piece.size)
			}

			if uint64(block.Begin)+uint64(len(block.Data)) > uint64(len(piece.buf)) {
				return errors.New("wrong piece block: block data overflows piece.")
			}

			copy(piece.buf[block.Begin:], block.Data)

			peer.reqBacklog--
			piece.downloaded += uint(len(block.Data))
		}
	}

	return nil
}

func startPiecesDownload(torr *Torrent, peersChan chan *PeerConn, workCtx context.Context) chan *PieceProgress {
	piecesChan := genPiecesProgresses(torr)
	donePieces := make(chan *PieceProgress, len(torr.PiecesHashes))
	donePiecesTotal := atomic.Uint64{}

	select {
	case <-workCtx.Done():
		close(piecesChan)
		close(donePieces)
		return donePieces
	default:
	}

	for p := range peersChan {
		peer := p
		go func() {
			if err := peer.sendUnchoke(); err != nil {
				logrus.Warnf("peer %s couldn't get unchoked: %s", peer.peer.String(), err.Error())
				return
			}

			if err := peer.sendInterestedMsg(); err != nil {
				logrus.Warnf("peer %s couldn't send 'interested': %s", peer.peer.String(), err.Error())
				return
			}

			for pieceProgress := range piecesChan {
				if peer.bitfield == nil || !peer.bitfield.HasPiece(pieceProgress.index) {
					piecesChan <- pieceProgress
					continue
				}

				if err := attemptPieceDownload(peer, pieceProgress); err != nil {
					logrus.Warnf("peer %s couldn't download piece %d: %s", peer.peer.String(), pieceProgress.index, err.Error())
					piecesChan <- pieceProgress
					continue
				}

				if err := pieceProgress.ValidateHash(); err != nil {
					logrus.Warnf("piece %d invalid: %s. retrying", pieceProgress.index, err.Error())
					pieceProgress.requested = 0
					pieceProgress.downloaded = 0
					piecesChan <- pieceProgress
					continue
				}

				donePieces <- pieceProgress
				donePiecesTotal.Add(1)

				percent := float64(donePiecesTotal.Load()) / float64(len(torr.PiecesHashes)) * 100
				fmt.Printf("(%0.2f%%) Downloaded piece #%d\n", percent, pieceProgress.index)
			}
		}()
	}

	return donePieces
}
