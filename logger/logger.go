package logger

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

type LoggerOpts struct {
	LogSentMessages bool
	LogRecvMessages bool
}

var opts LoggerOpts

func SetupLoggerOpts(level string, sentMsgs, recvMsgs bool) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("failed to parse level: %w", err)
	}

	logrus.SetLevel(l)

	opts = LoggerOpts{
		LogSentMessages: sentMsgs,
		LogRecvMessages: recvMsgs,
	}

	return nil
}

func LogSentMessage(format string, args ...any) {
	if !opts.LogSentMessages {
		return
	}

	logrus.Debugf(format, args...)
}

func LogRecvMessage(format string, args ...any) {
	if !opts.LogRecvMessages {
		return
	}

	logrus.Debugf(format, args...)
}
