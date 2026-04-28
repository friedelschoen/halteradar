package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-xmlfmt/xmlfmt"
	zmq "github.com/pebbe/zmq4"
)

const endpoint = "tcp://pubsub.besteffort.ndovloket.nl:7658"

var topics = []string{
	"/QBUZZ/KV15messages",
	"/QBUZZ/KV17cvlinfo",
	"/QBUZZ/KV6posinfo",
}

func main() {
	ctx, err := zmq.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Term()

	sub, err := ctx.NewSocket(zmq.SUB)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()

	for _, topic := range topics {
		if err := sub.SetSubscribe(topic); err != nil {
			log.Fatalf("subscribe %s: %v", topic, err)
		}
	}

	if err := sub.Connect(endpoint); err != nil {
		log.Fatal(err)
	}

	log.Printf("connected to %s", endpoint)
	for _, topic := range topics {
		log.Printf("subscribed to %s", topic)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	poller := zmq.NewPoller()
	poller.Add(sub, zmq.POLLIN)

	for {
		select {
		case <-interrupt:
			log.Println("stopping")
			return
		default:
		}

		sockets, err := poller.Poll(1 * time.Second)
		if err != nil {
			log.Println("poll:", err)
			continue
		}

		for _, socket := range sockets {
			if socket.Socket != sub {
				continue
			}

			parts, err := sub.RecvMessageBytes(0)
			if err != nil {
				log.Println("recv:", err)
				continue
			}

			if len(parts) < 2 {
				continue
			}

			topic := string(parts[0])

			fmt.Printf("\n--- %s ---\n", topic)

			unzipped, _ := gzip.NewReader(bytes.NewBuffer(parts[1]))
			c, _ := io.ReadAll(unzipped)
			fmt.Printf("%s\n", xmlfmt.FormatXML(string(c), "", "  "))
			if len(parts) > 2 {
				fmt.Printf("\n%d more parts...\n", len(parts)-2)
			}
		}
	}
}
