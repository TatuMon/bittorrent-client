package torrents

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type MessageID uint8

const (
	MsgChoke MessageID = iota
	MsgUnchoke
	MsgInterested
	MsgNotInterested
	MsgHave
	MsgBitField
	MsgRequest
	MsgPiece
	MsgCancel
	MsgPort
)

type Message struct {
	ID      MessageID
	Payload []byte
}

/**
BitTorrent messages follow the format: <lenght prefix><message ID><payload>
where <lenght prefix> is 4 bytes long, <message ID> is a single byte long and <payload>'s lenght
is equal to <length prefix> - 1
*/
func (m *Message) Serialize() []byte {
	var buf bytes.Buffer

	msgLen := uint32(1 + len(m.Payload))
	lenPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lenPrefix, msgLen)

	buf.Write(lenPrefix[:])
	buf.WriteByte(byte(m.ID))
	buf.Write(m.Payload)

	return buf.Bytes()
}

/**
BitTorrent messages follow the format: <lenght prefix><message ID><payload>
where <lenght prefix> is 4 bytes long, <message ID> is a single byte long and <payload>'s lenght
is equal to <length prefix> - 1

If both the returned pointes are nil (*Message and error), one MUST consider the message as a keep-alive
*/
func MessageFromStream(d io.Reader) (*Message, error) {
	msgLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(d, msgLenBuf); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}
	msgLen := binary.BigEndian.Uint32(msgLenBuf)

	if msgLen == 0 {
		return nil, nil
	}
	
	msgID := make([]byte, 1)
	if _, err := io.ReadFull(d, msgID); err != nil {
		return nil, fmt.Errorf("failed to read message id: %w", err)
	}

	msgPayload := make([]byte, msgLen - 1)
	if _, err := io.ReadFull(d, msgPayload); err != nil {
		return nil, fmt.Errorf("failed to read message payload: %w", err)
	}

	return &Message{
		ID: MessageID(msgID[0]),
		Payload: msgPayload,
	}, nil
}

type Bitfield []byte

func (b Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	bitIndex := index - (8 * byteIndex)

	return b[byteIndex]&(1<<bitIndex) != 0
}
