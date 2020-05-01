package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/scgolang/osc"
)

type grid struct {
	mtx   sync.Mutex
	conn  osc.Conn
	field []byte
}

func newGrid(onKey func(x, y int, down bool)) (*grid, error) {
	addr, err := findGridAddr(context.Background())
	if err != nil {
		return nil, err
	}

	conn, err := connect(context.Background(), addr)
	if err != nil {
		return nil, err
	}

	localPort, err := getLocalPort(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	go func() {
		err := conn.Serve(1, osc.PatternMatching{
			"/sys/id":          osc.Method(func(msg osc.Message) error { return nil }),
			"/sys/prefix":      osc.Method(func(msg osc.Message) error { return nil }),
			"/monome/grid/key": onKeyMethod(onKey),
		})
		log.Fatalf("conn.Serve: %v", err)
	}()

	for _, msg := range []osc.Message{
		{Address: "/sys/host", Arguments: []osc.Argument{osc.String("localhost")}},
		{Address: "/sys/port", Arguments: []osc.Argument{osc.Int(localPort)}},
		{Address: "/sys/prefix", Arguments: []osc.Argument{osc.String("/monome")}},
	} {
		if err := conn.Send(msg); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return &grid{
		conn:  conn,
		field: make([]byte, 16*8), // (x, y) = [y*16 + x]
	}, nil
}

func (g *grid) clear() {
	for x := 0; x < 16; x++ {
		for y := 0; y < 8; y++ {
			g.set(x, y, 0)
		}
	}
	g.update()
}

func (g *grid) set(x, y, z int) {
	x %= 16
	y %= 8
	z %= 16

	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.field[x+y*16] = byte(z)
}

func (g *grid) update() error {
	s1 := []osc.Argument{osc.Int(0), osc.Int(0)}
	s2 := []osc.Argument{osc.Int(8), osc.Int(0)}

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			i := x + y*16
			s1 = append(s1, osc.Int(g.field[i]))
			s2 = append(s2, osc.Int(g.field[i+8]))
		}
	}

	for _, msg := range []osc.Message{
		{Address: "/monome/grid/led/level/map", Arguments: s1},
		{Address: "/monome/grid/led/level/map", Arguments: s2},
	} {
		if err := g.conn.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func onKeyMethod(onKey func(x, y int, down bool)) osc.Method {
	return func(msg osc.Message) error {
		if n := len(msg.Arguments); n < 3 {
			return fmt.Errorf("unexpected argument count %d", n)
		}
		x, err := msg.Arguments[0].ReadInt32()
		if err != nil {
			return fmt.Errorf("error reading x: %w", err)
		}
		y, err := msg.Arguments[1].ReadInt32()
		if err != nil {
			return fmt.Errorf("error reading y: %w", err)
		}
		z, err := msg.Arguments[2].ReadInt32()
		if err != nil {
			return fmt.Errorf("error reading z: %w", err)
		}
		onKey(int(x), int(y), z > 0)
		return nil
	}
}

