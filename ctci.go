package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/lunixbochs/struc"
)

type ctciHDR struct {
	MsgLen int `struc:"int16,little"`
	Seq    int `struc:"int32,little"`
}
type ctciType struct {
	Hdr string `struc:"[2]byte,little"`
}
type ctciLR struct {
	//Hdr          [2]byte `struc:"[2]byte,little"`
	Hdr          string `struc:"[2]byte,little"`
	LastRecvSeq  int    `struc:"int32,little"`
	LastRecvSeq2 int    `struc:"int32,little"`
	StartFlag    int    `struc:"int32,little"`
}

type ctciER struct {
	//Hdr  [2]byte `struc:"[2]byte,little"`
	Hdr  string `struc:"[2]byte,little"`
	Code int    `struc:"int32,little"`
}

type ctciAK struct {
	Hdr         [2]byte `struc:"[2]byte,little"`
	LastRecvSeq uint32  `struc:"uint32,little"`
}

type ctciXX struct {
	Hdr  string `struc:"[2]byte,little"`
	Body string
}

type Conn struct {
	conn         net.Conn
	outgoing     chan []byte
	incoming     chan []byte
	messageMutex sync.Mutex
}

func (ctci *Conn) writer() {
	for {
		select {
		case out := <-ctci.outgoing:
			var hdr ctciHDR
			hdr.MsgLen = len(out) + 4 // Seq
			hdr.Seq = 0
			var bb bytes.Buffer
			struc.Pack(&bb, &hdr)
			//fmt.Printf("=> % X % X\n", bb.Bytes(), out)
			ctci.conn.Write(bb.Bytes())
			ctci.conn.Write(out)
		}
	}
}

func (ctci *Conn) reader() {
	connReader := bufio.NewReader(ctci.conn)
	for {
		//i, err := connReader.ReadString('\n')
		//bLen := make([]byte, 2)
		//i, err := io.ReadFull(connReader, len)
		var nMsgLen uint16
		err := binary.Read(connReader, binary.LittleEndian, &nMsgLen)
		if err != nil {
			log.Fatalln("incoming error1:", err)
			return
		}
		//nMsgLen := binary.LittleEndian.Uint16(bLen)

		if nMsgLen > 1024 {
			log.Fatalln("incoming error with Length:", nMsgLen)
		}

		data := make([]byte, nMsgLen)
		err = binary.Read(connReader, binary.LittleEndian, data)
		if err != nil {
			log.Fatalln("incoming error2:", err)
			return
		}
		//fmt.Printf("% X\n", i)
		//incoming <- i
		//fmt.Printf("%d: % X\n", i, len)
		//fmt.Printf("<= %0X % X\n", nMsgLen, data)
		//seq = binary.LittleEndian.Uint32(data)
		ctci.incoming <- data[4:]
	}
}

func (ctci *Conn) acker() {
	for {
		time.Sleep(15 * time.Second)
		var ak ctciAK
		var bb bytes.Buffer
		ak.Hdr[0] = 'A'
		ak.Hdr[1] = 'K'
		ak.LastRecvSeq = 0
		struc.Pack(&bb, &ak)
		ctci.outgoing <- bb.Bytes()
		log.Println("==> AK")
	}
}

func (ctci *Conn) handshake() (result bool) {
	// SEND LR
	var lr ctciLR
	//lr.Hdr[0] = 'L'
	//lr.Hdr[1] = 'R'
	lr.Hdr = "LR"
	lr.LastRecvSeq = -1
	lr.LastRecvSeq2 = -1
	lr.StartFlag = 3
	var bb bytes.Buffer
	struc.Pack(&bb, &lr)
	ctci.outgoing <- bb.Bytes()

	handshaked := false
	for {
		if handshaked {
			break
		}
		select {
		case in := <-ctci.incoming:
			// Process Messages
			msgType := fmt.Sprintf("%c%c", in[0], in[1])
			if msgType == "ER" {
				var msg ctciER
				var bb bytes.Buffer
				bb.Reset()
				binary.Write(&bb, binary.LittleEndian, in)
				struc.Unpack(&bb, &msg)
				log.Println("[Msg]", msg)
			} else if msgType == "LR" {
				var msg ctciLR
				var bb bytes.Buffer
				binary.Write(&bb, binary.LittleEndian, in)
				struc.Unpack(&bb, &msg)
				log.Println("[Msg]", msg)
				{
					var er ctciER
					var bb bytes.Buffer
					//er.Hdr[0] = 'E'
					//er.Hdr[1] = 'R'
					er.Hdr = "ER"
					er.Code = 0
					struc.Pack(&bb, &er)
					ctci.outgoing <- bb.Bytes()
				}
				handshaked = true
			}
		}
	}

	return true

}

func Dial(network, addr string) (*Conn, error) {
	//c, err := net.DialTimeout(network, addr, DefaultTimeout)
	c, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	conn := NewConn(c)
	conn.Start()
	return conn, nil
}

func NewConn(conn net.Conn) *Conn {
	return &Conn{
		conn:     conn,
		outgoing: make(chan []byte, 128),
		incoming: make(chan []byte, 128),
	}
}

// Start initializes goroutines to read responses and process messages
func (ctci *Conn) Start() {
	go ctci.reader()
	go ctci.writer()
	if ctci.handshake() != true {
		return
	}
	go ctci.acker()
}

func main() {

	ctci, err := Dial("tcp", "10.138.23.74:8405")
	if err != nil {
		log.Fatalln(err)
	}
	defer ctci.conn.Close()

	for {
		select {
		case in := <-ctci.incoming:
			//fmt.Printf("% X \n", in)

			// Process Messages
			msgType := fmt.Sprintf("%c%c", in[0], in[1])
			if msgType == "ER" {
				var msg ctciER
				var bb bytes.Buffer
				bb.Reset()
				binary.Write(&bb, binary.LittleEndian, in)
				struc.Unpack(&bb, &msg)
				log.Println("[Msg]", msg)
			} else if msgType == "LR" {
				var msg ctciLR
				var bb bytes.Buffer
				binary.Write(&bb, binary.LittleEndian, in)
				struc.Unpack(&bb, &msg)
				log.Println("[Msg]", msg)
				//seq := uint32(msg.LastRecvSeq)
				{
					var er ctciER
					var bb bytes.Buffer
					//er.Hdr[0] = 'E'
					//er.Hdr[1] = 'R'
					er.Hdr = "ER"
					er.Code = 0
					struc.Pack(&bb, &er)
					ctci.outgoing <- bb.Bytes()
				}
			} else {
				log.Println("[Msg]", string(in))
			}
		}
	}

}
