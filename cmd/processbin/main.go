// Command processbin is a CLI to emulate process behaviors
// such as logging enormous amount of data, crashing etc
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
)

const (
	DefaultLogLineSize = 150
	SigtermExitCode    = 143
	SigintExitCode     = 130
	ReadTimeout        = 5 * time.Second
)

func log(logLine string, logErr, structured bool) {
	if structured {
		m := map[string]string{
			"key": logLine,
		}
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(os.Stdout, "%s\n", b)
		if logErr {
			fmt.Fprintf(os.Stderr, "%s\n", b)
		}
	} else {
		fmt.Fprintf(os.Stdout, "%v\n", logLine)
		if logErr {
			fmt.Fprintf(os.Stderr, "%v\n", logLine)
		}
	}
}

func startHttpServer(wg *sync.WaitGroup, port int64) *http.Server {
	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), ReadTimeout: ReadTimeout}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, "hello from processbin\n")
		if err != nil {
			panic(err)
		}
	})

	go func() {
		defer wg.Done()
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return srv
}

func main() {
	app := &cli.App{
		Name:        "processbin",
		Usage:       "Used to test edge cases in IPC",
		Description: "A test binary that can be tuned to generate edge cases for IPC.",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:    "exit-after",
				Aliases: []string{"t"},
				Value:   0,
				Usage:   "Time in seconds until the program exists. If set to 0 the program never ends",
			},
			&cli.BoolFlag{
				Name:    "ignore-signals",
				Aliases: []string{"i"},
				Usage:   "If this switch is set then the program ignores interrupts",
			},
			&cli.IntFlag{
				Name:    "signal-timeout",
				Aliases: []string{"o"},
				Usage:   "The time to wait between responding to a SIGINT",
			},
			&cli.StringFlag{
				Name:    "log-fixed",
				Aliases: []string{"f"},
				Usage:   "Log value",
			},
			&cli.Int64Flag{
				Name:    "log-size",
				Aliases: []string{"l"},
				Usage:   "Log a random string with given size.",
				Value:   DefaultLogLineSize,
			},
			&cli.Int64Flag{
				Name:    "log-interval, k",
				Aliases: []string{"k"},
				Usage:   "Interval at which the program is writing logs to stdout",
				Value:   1,
			},
			&cli.BoolFlag{
				Name:    "log-error",
				Aliases: []string{"e"},
				Usage:   "If this switch is set then the program writes all the logs in stderr as well as stdout.",
			},
			&cli.BoolFlag{
				Name:    "log-structured",
				Aliases: []string{"j"},
				Usage:   "If this switch is set then the program writes all the logs in json format.",
				Value:   false,
			},
			&cli.Int64Flag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "port processbin will listen to for http requests. Useful for testing proxy mode.",
				Value:   0,
			},
		},
		Action: func(c *cli.Context) error {
			osCh := make(chan os.Signal, 1)
			signal.Notify(osCh, syscall.SIGTERM, syscall.SIGINT)
			defer func() {
				signal.Stop(osCh)
			}()

			var logLine string
			if len(c.String("log-fixed")) > 0 {
				logLine = c.String("log-fixed")
			} else {
				logBytes := make([]byte, c.Int64("log-size"))
				returnedBytes, err := rand.Read(logBytes)
				if err != nil {
					panic(err)
				}
				logLine = hex.EncodeToString(logBytes[:returnedBytes])
			}

			timeoutCh := make(chan struct{}, 1)
			exitAfter := c.Int64("exit-after")
			if exitAfter > 0 {
				go func() {
					<-time.After(time.Duration(exitAfter) * time.Second)
					timeoutCh <- struct{}{}
				}()
			}

			logCh := make(chan struct{}, 1)
			logTimeoutFunc := func() {
				<-time.After(time.Duration(c.Int64("log-interval")) * time.Second)
				logCh <- struct{}{}
			}
			log(logLine, c.Bool("log-error"), c.Bool("log-structured"))
			go logTimeoutFunc()

			port := c.Int64("port")
			var srv *http.Server
			if port != 0 {
				httpServerExitDone := &sync.WaitGroup{}
				httpServerExitDone.Add(1)
				srv = startHttpServer(httpServerExitDone, port)
			}

			for {
				select {
				case s := <-osCh:
					if srv != nil {
						if err := srv.Shutdown(context.TODO()); err != nil {
							return err
						}
					}

					if !c.Bool("ignore-signals") {
						timeout := c.Int("signal-timeout")
						fmt.Fprintf(os.Stdout, "Caught SIGINT, waiting for %d seconds before exiting\n", timeout)
						<-time.After(time.Duration(timeout) * time.Second)
						if s == syscall.SIGTERM {
							os.Exit(SigtermExitCode)
						} else {
							os.Exit(SigintExitCode)
						}
					}
				case <-timeoutCh:
					if srv != nil {
						if err := srv.Shutdown(context.TODO()); err != nil {
							return err
						}
					}
					return nil
				case <-logCh:
					log(logLine, c.Bool("log-error"), c.Bool("log-structured"))
					go logTimeoutFunc()
				}
			}

		},
	}

	err := app.Run(os.Args)
	if err != nil {
		return
	}

}
