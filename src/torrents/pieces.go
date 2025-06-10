package torrents

import (
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

func (p *PieceProgress) downloadPiece(peer *PeerConn) error {
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

	for pieceProgress := range pChan {
		go func() {
			peer := <-peersConns
			err := pieceProgress.downloadPiece(peer)
			if err != nil {
				logrus.Warnf("failed to download piece %d: %s", pieceProgress.index, err.Error())
				pChan <- pieceProgress
				return
			}

			if pieceProgress.hasFinished() {
				pieceProgress.writeTargetFile(torr.FileName)
				finishedPieces++
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
