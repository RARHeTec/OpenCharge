// Command ykc-sim is a virtual YunKuaiChong V1.6 charging pile for integration
// testing. It connects to the gateway as a real pile would and walks through
// login, heartbeat, telemetry and a transaction record.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8768", "gateway TCP address")
	device := flag.String("device", "55031412782305", "device id (14 BCD digits)")
	gun := flag.Int("gun", 1, "gun number")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	fmt.Printf("connected to %s as device %s gun %d\n", *addr, *device, *gun)

	seq := uint16(0)
	send := func(typ byte, body []byte) {
		seq++
		wire := protocol.Encode(seq, false, typ, body)
		fmt.Printf("-> type=0x%02X seq=%d len=%d hex=%X\n", typ, seq, len(wire), wire)
		if _, err := conn.Write(wire); err != nil {
			log.Fatal(err)
		}
	}
	recv := func() protocol.Frame {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		s := protocol.NewSplitter(conn)
		wire, err := s.NextFrame()
		if err != nil {
			log.Fatalf("recv: %v", err)
		}
		f, err := protocol.Decode(wire)
		if err != nil {
			log.Fatalf("decode: %v", err)
		}
		fmt.Printf("<- type=0x%02X seq=%d body=%X\n", f.Type, f.Seq, f.Body)
		return f
	}

	deviceBCD := bcd14(*device)
	// 1. login
	login := make([]byte, 30)
	copy(login[:7], deviceBCD)
	login[7] = 0x00 // DC pile
	login[8] = 0x02 // 2 guns
	login[9] = 0x10 // V1.6
	copy(login[10:18], []byte("v1.6.0\x00\x00"))
	login[18] = 0x01 // LAN
	login[29] = 0x04 // operator other
	send(protocol.TypeLogin, login)
	recv() // login ack

	// 2. heartbeat
	hb := append(append([]byte(nil), deviceBCD...), byte(*gun), 0x00)
	send(protocol.TypeHeartbeat, hb)
	recv() // heartbeat ack

	// 3. realtime telemetry: charging at 7.2 kW, 100.0 V x 72.0 A
	rt := make([]byte, 69)
	txn := bcd16("0000000000000001")
	copy(rt[:16], txn)
	copy(rt[16:23], deviceBCD)
	rt[23] = byte(*gun)
	rt[24] = 0x03 // charging
	rt[25] = 0x01 // returned
	rt[26] = 0x01 // plugged
	binary.LittleEndian.PutUint16(rt[27:29], 1000) // 100.0 V
	binary.LittleEndian.PutUint16(rt[29:31], 720)  // 72.0 A
	binary.LittleEndian.PutUint32(rt[46:50], 5000) // 0.5 kWh
	binary.LittleEndian.PutUint32(rt[54:58], 3000) // 0.3 yuan
	send(protocol.TypeRealtime, rt)

	// 4. transaction record: 0.5 kWh @ 0.6000 yuan/kWh = 0.3000 yuan
	txnBody := make([]byte, 158)
	copy(txnBody[:16], txn)
	copy(txnBody[16:23], deviceBCD)
	txnBody[23] = byte(*gun)
	binary.LittleEndian.PutUint32(txnBody[38:42], 60000) // flat price 0.60000
	binary.LittleEndian.PutUint32(txnBody[42:46], 5000)  // 0.5 kWh
	binary.LittleEndian.PutUint32(txnBody[50:54], 3000)   // sharp segment amount 0.3 yuan
	binary.LittleEndian.PutUint32(txnBody[112:116], 5000)
	binary.LittleEndian.PutUint32(txnBody[120:124], 3000) // 0.3 yuan
	txnBody[141] = 0x01 // app start
	send(protocol.TypeTransaction, txnBody)
	recv() // transaction ack

	fmt.Println("flow complete")
	time.Sleep(500 * time.Millisecond)
}

// bcd14 packs a 14-digit decimal string into 7 BCD bytes.
func bcd14(s string) []byte {
	out := make([]byte, 7)
	for i := 0; i < 7 && i*2+1 < len(s); i++ {
		hi, lo := s[i*2]-'0', s[i*2+1]-'0'
		out[i] = (lo << 4) | hi
	}
	return out
}

func bcd16(s string) []byte {
	out := make([]byte, 16)
	for i := 0; i+1 < len(s) && i/2 < 16; i += 2 {
		hi, lo := s[i]-'0', s[i+1]-'0'
		out[i/2] = (lo << 4) | hi
	}
	return out
}
