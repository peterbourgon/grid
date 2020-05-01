package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/scgolang/osc"
)

func main() {
	g, err := newGrid(func(x, y int, down bool) {
		if down {
			log.Printf("<%d %d> ↓", x, y)
		} else {
			log.Printf("<%d %d> ↑", x, y)
		}
	})
	if err != nil {
		log.Fatal(err)
	}

	g.clear()       //
	defer g.clear() // boy scout rules

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.set(rand.Intn(16), rand.Intn(8), rand.Intn(16))
			g.update()
		case sig := <-sigc:
			log.Printf("%s", sig)
			return
		}
	}
}

const serialoscAddr = "localhost:12002"

func connect(ctx context.Context, addr string) (osc.Conn, error) {
	laddr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		return nil, err
	}

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := osc.DialUDPContext(ctx, "udp", laddr, raddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func getLocalPort(conn osc.Conn) (int, error) {
	if conn == nil {
		return 0, fmt.Errorf("nil connection")
	}

	_, localPortStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return 0, err
	}

	localPort, err := strconv.Atoi(localPortStr)
	if err != nil {
		return 0, err
	}

	return localPort, nil
}

func findGridAddr(ctx context.Context) (string, error) {
	conn, err := connect(ctx, serialoscAddr)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localPort, err := getLocalPort(conn)
	if err != nil {
		return "", err
	}

	var (
		portc = make(chan int)
		errc  = make(chan error)
	)
	go func() {
		errc <- conn.Serve(1, findGridPortDispatcher(portc))
	}()

	if err := conn.Send(osc.Message{
		Address: "/serialosc/list",
		Arguments: []osc.Argument{
			osc.String("localhost"),
			osc.Int(localPort),
		},
	}); err != nil {
		return "", err
	}

	select {
	case port := <-portc:
		return net.JoinHostPort("localhost", fmt.Sprint(port)), nil
	case <-time.After(time.Second):
		return "", fmt.Errorf("timeout")
	case err := <-errc:
		return "", err
	}
}

func findGridPortDispatcher(c chan<- int) osc.Dispatcher {
	return osc.Dispatcher(osc.PatternMatching{
		"/serialosc/device": osc.Method(func(msg osc.Message) error {
			if n := len(msg.Arguments); n%3 != 0 {
				return fmt.Errorf("unexpected number of arguments: %d", n)
			}

			for i := 0; i < len(msg.Arguments); i += 3 {
				s, err := msg.Arguments[i+1].ReadString()
				if err != nil {
					continue
				}

				if s != "monome 128" {
					continue
				}

				p, err := msg.Arguments[i+2].ReadInt32()
				if err != nil {
					continue
				}

				c <- int(p)
			}

			return nil
		}),
	})
}
