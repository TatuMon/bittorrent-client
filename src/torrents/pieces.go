package torrents

import (
	"errors"
	"fmt"
	"sync"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

// 16KB maximum for compatibility: https://wiki.theory.org/BitTorrentSpecification#request:_.3Clen.3D0013.3E.3Cid.3D6.3E.3Cindex.3E.3Cbegin.3E.3Clength.3E
const blockSize = 16 * 1024
const maxPipelinedRequests = 5

/*
*
PieceProgress represents the download process of a single piece.

A single PieceProgress MUST be handled by at most ONE worker goroutine
*/
type PieceProgress struct {
	index int
	buf []byte
	expectedHash Sha1Checksum
	completed bool
}

func (p *PieceProgress) hasFinished() bool {
	return false
}

func (p *PieceProgress) writeTargetFile(filepath string) error {
	return nil
}

func (p *PieceProgress) requestPiece(peer *PeerConn) error {
	if !peer.bitfield.HasPiece(p.index) {
		return errors.New(fmt.Sprintf("peer doesn't have piece %d", p.index))
	}

	

	return nil
}

func newPieceProgress(index int, expectedHash Sha1Checksum, pieceSize uint) *PieceProgress {
	return &PieceProgress{
		index: index,
		buf:   make([]byte, pieceSize),
		expectedHash: expectedHash,
		completed: false,
	}
}

func genPiecesProgresses(torr *Torrent) chan *PieceProgress {
	channel := make(chan *PieceProgress, len(torr.PiecesHashes))

	for i, hash := range torr.PiecesHashes {
		pp := newPieceProgress(i, hash, torr.calculatePieceSize(uint(i)))
		channel <- pp
	}

	return channel
}

func startPiecesDownload(torr *Torrent, peersConns chan *PeerConn) {
	pChan := genPiecesProgresses(torr)
	finishedPieces := 0

	var wg sync.WaitGroup
	wg.Add(len(torr.PiecesHashes))

	for pp := range pChan {
		pieceProgress := pp
		go func() {
			peer := <-peersConns

			msg, err := peer.read()
			if err != nil {
				logrus.Warnf("failed to read from connection %s: %s", peer.peer.String(), err.Error())
				peersConns <- peer
				pChan <- pieceProgress
				return
			}

			if !peer.unchoked {
				peersConns <- peer
				pChan <- pieceProgress
				return
			}

			if !peer.interested {
				if err := peer.sendInterestedMsg(); err != nil {
					logrus.Warnf("failed to unchoke: %s. dropping connection.", err.Error())
					peer.conn.Close()
					pChan <- pieceProgress
					return
				}
			}

			for peer.reqBacklog < maxReqBacklog {
				peer.reqBacklog++

				err := pieceProgress.requestPiece(peer)
				if err != nil {
					logrus.Warnf("failed to download piece %d: %s", pieceProgress.index, err.Error())
					peersConns <- peer
					pChan <- pieceProgress
					return
				}

				if pieceProgress.hasFinished() {
					pieceProgress.writeTargetFile(torr.FileName)
					finishedPieces++
				}
			}

			if finishedPieces == len(torr.PiecesHashes) {
				close(pChan)
			}

			color.Green("piece %d downloaded successfuly", pieceProgress.index)
			wg.Done()
		}()
	}

	wg.Wait()
	color.Green("download finishes successfuly")
}
