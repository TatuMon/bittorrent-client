package torrents

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"

	bencode "github.com/jackpal/bencode-go"
	"github.com/sirupsen/logrus"
)

type Sha1Checksum [20]byte

type bencodeTorrentInfo struct {
	Length      uint   `bencode:"length"` // Length of the final file in bytes
	Name        string `bencode:"name"`
	PieceLength uint   `bencode:"piece length"` // Number of bytes in each piece
	Pieces      string `bencode:"pieces"`       // String consisting of the concatenation of all 20-byte SHA1 hash values, one per piece (byte string, i.e. not urlencoded)
}

type bencodeTorrent struct {
	Announce     string             `bencode:"announce"`
	Info         bencodeTorrentInfo `bencode:"info"`
	Comment      string             `bencode:"comment"`
	CreationDate int                `bencode:"creation date"`
	CreatedBy    string             `bencode:"created by"`
}

type Torrent struct {
	Announce     string
	Comment      string
	CreationDate int
	CreatedBy    string
	FileSize     uint
	FileName     string
	PieceSize    uint
	PiecesHashes []Sha1Checksum
	InfoHash     Sha1Checksum
	TotalPieces int
}

func (t *Torrent) calculatePieceSize(index uint) uint {
	begin, end := t.calculateBoundsForPiece(int(index))
	return uint(end - begin)
}

func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * int(t.PieceSize)
	end = min(begin + int(t.PieceSize), int(t.FileSize))
	return begin, end
}

func (t *Torrent) JsonPreviewIndented() (string, error) {
	t.PiecesHashes = make([]Sha1Checksum, 0)
	j, err := json.MarshalIndent(t, "", "\t")
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal torrent file: %w", err)
	}

	return string(j), nil
}


func TorrentFromFile(torrentPath string) (*Torrent, error) {
	torrentFile, err := os.Open(torrentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w\n", err)
	}

	torr, err := getTorrentFile(torrentFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse torrent file: %w\n", err)
	}

	return torr, nil
}

func genInfoHash(t bencodeTorrentInfo) (Sha1Checksum, error) {
	buf := new(bytes.Buffer)
	if err := bencode.Marshal(buf, t); err != nil {
		return Sha1Checksum{}, fmt.Errorf("failed to marshal field 'info': %w", err)
	}

	checksum := sha1.Sum(buf.Bytes())
	return checksum, nil
}

func torrentFromBencode(t bencodeTorrent) (*Torrent, error) {
	concatedHashes := []byte(t.Info.Pieces)
	chunks := len(concatedHashes) / 20

	if len(concatedHashes) % 20 != 0 {
		return nil, fmt.Errorf("received malformed pieces hashes")
	}

	pHashes := make([]Sha1Checksum, chunks)
	for i := range chunks {
		copy(pHashes[i][:], concatedHashes[i*20:(i+1)*20])
	}

	infoHash, err := genInfoHash(t.Info)
	if err != nil {
		return nil, fmt.Errorf("failed to generate sha1 checksum of field 'info': %w", err)
	}

	return &Torrent{
		Announce:     t.Announce,
		Comment:      t.Comment,
		CreationDate: t.CreationDate,
		CreatedBy:    t.CreatedBy,
		FileSize:     t.Info.Length,
		FileName:     t.Info.Name,
		PieceSize:    t.Info.PieceLength,
		PiecesHashes: pHashes,
		InfoHash:     infoHash,
	}, nil
}

func getTorrentFile(torrentFile *os.File) (*Torrent, error) {
	tData := bencodeTorrent{}
	if err := bencode.Unmarshal(torrentFile, &tData); err != nil {
		fmt.Fprintf(os.Stderr, "failed to unmarshal torrent file: %s\n", err.Error())
		os.Exit(1)
	}

	torrent, err := torrentFromBencode(tData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse torrent information: %s\n", err.Error())
		os.Exit(1)
	}

	torrent.TotalPieces = len(torrent.PiecesHashes)

	return torrent, nil
}

func StartDownload(torr *Torrent) error {
	peers, err := announce(torr)
	if err != nil {
		return fmt.Errorf("failed to announce to tracker: %w\n", err)
	}

	workCtx, _ := context.WithCancelCause(context.Background())

	peersConns := connectPeersAsync(torr, peers, workCtx)
	donePieces := startPiecesDownload(torr, peersConns, workCtx)

	for {
		select {
		case <-workCtx.Done():
			close(peersConns)
			fmt.Printf("download finished. cause: %s", context.Cause(workCtx).Error())
			return nil
		case p := <-donePieces:
			logrus.Debugf("piece %d downloaded", p.index)
		}
	}
}
