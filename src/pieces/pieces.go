package pieces

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/TatuMon/bittorrent-client/src/p2p"
	"github.com/TatuMon/bittorrent-client/src/torrent"
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

func PieceBlockFromMessage(msg *p2p.Message) (*PieceBlock, error) {
	if msg.ID != p2p.MsgPiece {
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
	expectedHash torrent.Sha1Checksum
	completed    bool
	requested    uint
	downloaded   uint
}

func newPieceProgress(index int, expectedHash torrent.Sha1Checksum, pieceSize uint) *PieceProgress {
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

func (p *PieceProgress) reset() {
	p.downloaded = 0
	p.requested = 0
}

func genPiecesProgresses(torr *torrent.Torrent) chan *PieceProgress {
	channel := make(chan *PieceProgress, len(torr.PiecesHashes))

	for i, hash := range torr.PiecesHashes {
		pp := newPieceProgress(i, hash, torr.CalculatePieceSize(uint(i)))
		channel <- pp
	}

	return channel
}

func attemptPieceDownload(peer *p2p.PeerConn, piece *PieceProgress) error {
	for piece.downloaded < piece.size {
		if peer.IsUnchoked() {
			for peer.ReqBacklog < p2p.MaxReqBacklog && piece.requested < piece.size {
				blockSize := piece.calcNextBlockSize()
				err := peer.SendRequestMsg(uint32(piece.index), uint32(piece.requested), uint32(blockSize))
				if err != nil {
					return fmt.Errorf("failed to request piece %d: %w", piece.index, err)
				}

				peer.ReqBacklog++
				piece.requested += blockSize
			}
		}

		msg, err := peer.Read()
		if err != nil {
			return fmt.Errorf("failed to read from peer: %w", err)
		}

		if msg != nil && msg.ID == p2p.MsgPiece {
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

			peer.ReqBacklog--
			piece.downloaded += uint(len(block.Data))
		}
	}

	return nil
}

func startPiecesDownload(torr *torrent.Torrent, peersChan chan *p2p.PeerConn, workCtx context.Context) chan *PieceProgress {
	piecesChan := genPiecesProgresses(torr)
	donePieces := make(chan *PieceProgress, torr.TotalPieces)
	donePiecesTotal := atomic.Uint64{}

	downloadCtx, downloadCtxCancel := context.WithCancel(workCtx)
	// Channel cleanup
	go func() {
		<-downloadCtx.Done()
		close(piecesChan)
		close(donePieces)
		logrus.Debug("all pieces downloaded")
	}()

	go func() {
		for p := range peersChan {
			peerConn := p
			go func() {
				peer := peerConn.GetPeer()

				if err := peerConn.SendUnchoke(); err != nil {
					logrus.Warnf("peer %s couldn't get unchoked: %s", peer.String(), err.Error())
					return
				}

				if err := peerConn.SendInterestedMsg(); err != nil {
					logrus.Warnf("peer %s couldn't send 'interested': %s", peer.String(), err.Error())
					return
				}

				keepAliveTicker := time.Tick(60 * time.Second)

				for {
					select {
					case pieceProgress := <-piecesChan:
						if pieceProgress == nil {
							return
						}

						bitfield := peerConn.GetBitfield()
						if bitfield == nil || !bitfield.HasPiece(pieceProgress.index) {
							piecesChan <- pieceProgress
							continue
						}

						if err := attemptPieceDownload(peerConn, pieceProgress); err != nil {
							logrus.Warnf("peer %s couldn't download piece %d: %s. closing connection", peer.String(), pieceProgress.index, err.Error())
							peerConn.CloseConn()
							piecesChan <- pieceProgress
							return
						}

						if err := pieceProgress.ValidateHash(); err != nil {
							logrus.Warnf("piece %d invalid: %s. retrying", pieceProgress.index, err.Error())
							pieceProgress.reset()
							piecesChan <- pieceProgress
							continue
						}

						donePieces <- pieceProgress
						donePiecesTotal.Add(1)

						percent := float64(donePiecesTotal.Load()) / float64(torr.TotalPieces) * 100
						fmt.Printf("(%0.2f%%) Downloaded piece #%d\n", percent, pieceProgress.index)

						if donePiecesTotal.Load() == uint64(torr.TotalPieces) {
							downloadCtxCancel()
							return
						}
					case <-keepAliveTicker:
						if err := peerConn.SendKeepAlive(); err != nil {
							logrus.Warnf("couldn't send 'keep alive' to peer %s: %s. closing connection", err.Error(), peer.String())
							return
						}
					case <-downloadCtx.Done():
						peerConn.CloseConn()
					}
				}
			}()
		}
	}()

	return donePieces
}

func writePiecesToFileAsync(filename string, pieces chan *PieceProgress, workctx context.Context) chan error {
	errchan := make(chan error, 1)

	file, err := os.Create(filename)
	if err != nil {
		errchan <- fmt.Errorf("failed to create output file: %w", err)
		close(errchan)
		return errchan
	}

	go func() {
		for p := range pieces {
			select {
			case <-workctx.Done():
				return
			default:
			}

			_, err := file.WriteAt(p.buf, int64(p.index*int(p.size)))
			if err != nil {
				errchan <- fmt.Errorf("failed to write to file: %w", err)
				return
			}
		}
		errchan <- fmt.Errorf("download completed")
		close(errchan)
	}()

	return errchan
}

func StartDownload(torr *torrent.Torrent, outFile string) error {
	peers, err := p2p.Announce(torr)
	if err != nil {
		return fmt.Errorf("failed to announce to tracker: %w\n", err)
	}

	workCtx, workCtxCancel := context.WithCancelCause(context.Background())

	peersConns := p2p.ConnectPeersAsync(torr, peers, workCtx)
	donePieces := startPiecesDownload(torr, peersConns, workCtx)
	writeErrChan := writePiecesToFileAsync(outFile, donePieces, workCtx)

	select {
	case <-workCtx.Done():
		fmt.Printf("download ended. cause: %s", context.Cause(workCtx).Error())
		workCtxCancel(nil) // line to satisfy `go vet`
	case writeErr := <-writeErrChan:
		workCtxCancel(writeErr)
		fmt.Println(writeErr.Error())
	}

	return nil
}
