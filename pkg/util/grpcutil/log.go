// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package grpcutil

import (
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/petermattis/goid"
	"golang.org/x/net/context"
	"google.golang.org/grpc/grpclog"

	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/syncutil"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
)

var discardLogger = grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, ioutil.Discard)

func init() {
	grpclog.SetLoggerV2(discardLogger)
}

// EnableLogging turns on relay of GRPC log messages to pkg/util/log.
// It must be called before GRPC is used.
func EnableLogging() {
	grpclog.SetLoggerV2(&logger{})
}

// NB: This interface is implemented by a pointer because using a value causes
// a synthetic stack frame to be inserted on calls to the interface methods.
// Specifically, we get a stack frame that appears as "<autogenerated>", which
// is not useful in logs.
//
// Also NB: we pass a depth of 2 here because all logging calls originate from
// the logging adapter file in grpc, which is an additional stack frame away
// from the actual logging site.
var _ grpclog.LoggerV2 = (*logger)(nil)

type logger struct{}

func (*logger) Info(args ...interface{}) {
	log.InfofDepth(context.TODO(), 2, "", args...)
}

func (*logger) Infoln(args ...interface{}) {
	log.InfofDepth(context.TODO(), 2, "", args...)
}

func (*logger) Infof(format string, args ...interface{}) {
	log.InfofDepth(context.TODO(), 2, format, args...)
}

func (*logger) Warning(args ...interface{}) {
	log.WarningfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Warningln(args ...interface{}) {
	log.WarningfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Warningf(format string, args ...interface{}) {
	if shouldPrint(transportFailedRe, connectionRefusedRe, time.Minute, format, args...) {
		log.WarningfDepth(context.TODO(), 2, format, args...)
	}
}

func (*logger) Error(args ...interface{}) {
	log.ErrorfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Errorln(args ...interface{}) {
	log.ErrorfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Errorf(format string, args ...interface{}) {
	log.ErrorfDepth(context.TODO(), 2, format, args...)
}

func (*logger) Fatal(args ...interface{}) {
	log.FatalfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Fatalln(args ...interface{}) {
	log.FatalfDepth(context.TODO(), 2, "", args...)
}

func (*logger) Fatalf(format string, args ...interface{}) {
	log.FatalfDepth(context.TODO(), 2, format, args...)
}

func (*logger) V(int) bool {
	// Proxying this to log.VDepth doesn't work because the argument type
	// to that function is unexported.
	return true
}

// https://github.com/grpc/grpc-go/blob/v1.7.0/clientconn.go#L937
var (
	transportFailedRe   = regexp.MustCompile("^" + regexp.QuoteMeta("grpc: addrConn.resetTransport failed to create client transport:"))
	connectionRefusedRe = regexp.MustCompile(
		strings.Join([]string{
			// *nix
			regexp.QuoteMeta("connection refused"),
			// Windows
			regexp.QuoteMeta("No connection could be made because the target machine actively refused it"),
			// Host removed from the network and no longer resolvable:
			// https://github.com/golang/go/blob/go1.8.3/src/net/net.go#L566
			regexp.QuoteMeta("no such host"),
		}, "|"),
	)
)

var spamMu = struct {
	syncutil.Mutex
	gids map[int64]time.Time
}{
	gids: make(map[int64]time.Time),
}

func shouldPrint(
	formatRe, argsRe *regexp.Regexp, freq time.Duration, format string, args ...interface{},
) bool {
	if formatRe.MatchString(format) {
		for _, arg := range args {
			if err, ok := arg.(error); ok {
				if argsRe.MatchString(err.Error()) {
					gid := goid.Get()
					now := timeutil.Now()
					spamMu.Lock()
					t, ok := spamMu.gids[gid]
					doPrint := !(ok && now.Sub(t) < freq)
					if doPrint {
						spamMu.gids[gid] = now
					}
					spamMu.Unlock()
					return doPrint
				}
			}
		}
	}
	return true
}
