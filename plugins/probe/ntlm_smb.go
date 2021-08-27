package probe

import (
	"bytes"
	"cube/log"
	"cube/model"
	"encoding/hex"
	"fmt"
	"github.com/JKme/go-ntlmssp"
	"net"
	"strings"
)

func SmbProbe(task model.ProbeTask) (result model.ProbeTaskResult) {
	result = model.ProbeTaskResult{ProbeTask: task, Result: "", Err: nil}
	realhost := fmt.Sprintf("%s:%v", task.Ip, task.Port)
	conn, err := net.DialTimeout("tcp", realhost, model.ConnectTimeout)
	if err != nil {
		log.Debug(err)
		return
	}
	_, err = conn.Write(NegotiateSMBv1Data1)
	if err != nil {
		return
	}
	r1, _ := readBytes(conn)

	//ff534d42 SMBv1的标示
	//fe534d42 SMBv2的标示
	//先发送探测SMBv1的payload，不支持的SMBv1的时候返回为空，然后尝试发送SMBv2的探测数据包
	//if hex.EncodeToString(r1[4:8]) == "ff534d42" {
	if len(r1) > 0 {
		_, err = conn.Write(NegotiateSMBv1Data2)
		if err != nil {
			return
		}

		ret, err := readBytes(conn)

		if err != nil || len(ret) < 45 {
			return
		}

		blob_length := uint16(bytes2Uint(ret[43:45], '<'))
		blob_count := uint16(bytes2Uint(ret[45:47], '<'))

		gss_native := ret[47:]
		off_ntlm := bytes.Index(gss_native, []byte("NTLMSSP"))
		//fmt.Println(off_ntlm)
		//fmt.Printf("GSS-NATIVE: %x\n", gss[off_ntlm:])
		//
		//fmt.Printf("NTLM: %x\n", gss[off_ntlm:blob_length])
		//fmt.Printf("native: %x\n", gss[int(blob_length):blob_count])
		native := gss_native[int(blob_length):blob_count]
		ss := strings.Split(string(native), "\x00\x00")
		//fmt.Println(ss)

		bs := gss_native[off_ntlm:blob_length]
		type2 := ntlmssp.ChallengeMsg{}
		tinfo := type2.String(bs)
		//fmt.Println(tinfo)

		NativeOS := TrimName(ss[0])
		NativeLM := TrimName(ss[1])
		//fmt.Println(NativeOS, NativeLM)
		tinfo += fmt.Sprintf("NativeOS: %s\nNativeLM: %s\n", NativeOS, NativeLM)
		result.Result = tinfo
	} else {
		conn2, _ := net.DialTimeout("tcp", realhost, model.ConnectTimeout)

		_, err = conn2.Write(NegotiateSMBv2Data1)

		if err != nil {
			fmt.Println(err)
			return
		}
		r2, _ := readBytes(conn2)

		var NTLMSSPNegotiatev2Data []byte
		if hex.EncodeToString(r2[70:71]) == "03" {
			flags := []byte{0x15, 0x82, 0x08, 0xa0}
			NTLMSSPNegotiatev2Data = getNTLMSSPNegotiateData(flags)
		} else {
			flags := []byte{0x05, 0x80, 0x08, 0xa0}
			NTLMSSPNegotiatev2Data = getNTLMSSPNegotiateData(flags)
		}

		_, err = conn2.Write(NegotiateSMBv2Data2)
		if err != nil {
			return
		}
		readBytes(conn2)

		_, err = conn2.Write(NTLMSSPNegotiatev2Data)
		ret, _ := readBytes(conn2)
		ntlmOff := bytes.Index(ret, []byte("NTLMSSP"))
		type2 := ntlmssp.ChallengeMsg{}
		tinfo := type2.String(ret[ntlmOff:])
		result.Result = tinfo
	}

	return result
}

func readBytes(conn net.Conn) (result []byte, err error) {
	buf := make([]byte, 4096)
	for {
		count, err := conn.Read(buf)
		if err != nil {
			break
		}
		result = append(result, buf[0:count]...)
		if count < 4096 {
			break
		}
	}
	return result, err
}

func TrimName(name string) string {
	return strings.TrimSpace(strings.Replace(name, "\x00", "", -1))
}
func bytes2Uint(bs []byte, endian byte) uint64 {
	var u uint64
	if endian == '>' {
		for i := 0; i < len(bs); i++ {
			u += uint64(bs[i]) << (8 * (len(bs) - i - 1))
		}
	} else {
		for i := 0; i < len(bs); i++ {
			u += uint64(bs[len(bs)-i-1]) << (8 * (len(bs) - i - 1))
		}
	}
	return u
}

//var NbtiosTCPData = []byte{
//	0x81,0x00,0x00,0x44,0x20,0x43,0x4b,0x46,0x44,0x45,0x4e,0x45,0x43,0x46,0x44,0x45,
//	0x46,0x46,0x43,0x46,0x47,0x45,0x46,0x46,0x43,0x43,0x41,0x43,0x41,0x43,0x41,0x43,
//	0x41,0x43,0x41,0x43,0x41,0x00,0x20,0x43,0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,
//	0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,0x41,0x43,
//	0x41,0x43,0x41,0x43,0x41,0x41,0x41,0x00,
//}

var NegotiateSMBv1Data1 = []byte{
	0x00, 0x00, 0x00, 0x85, 0xFF, 0x53, 0x4D, 0x42, 0x72, 0x00, 0x00, 0x00, 0x00, 0x18, 0x53, 0xC8,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFE,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x62, 0x00, 0x02, 0x50, 0x43, 0x20, 0x4E, 0x45, 0x54, 0x57, 0x4F,
	0x52, 0x4B, 0x20, 0x50, 0x52, 0x4F, 0x47, 0x52, 0x41, 0x4D, 0x20, 0x31, 0x2E, 0x30, 0x00, 0x02,
	0x4C, 0x41, 0x4E, 0x4D, 0x41, 0x4E, 0x31, 0x2E, 0x30, 0x00, 0x02, 0x57, 0x69, 0x6E, 0x64, 0x6F,
	0x77, 0x73, 0x20, 0x66, 0x6F, 0x72, 0x20, 0x57, 0x6F, 0x72, 0x6B, 0x67, 0x72, 0x6F, 0x75, 0x70,
	0x73, 0x20, 0x33, 0x2E, 0x31, 0x61, 0x00, 0x02, 0x4C, 0x4D, 0x31, 0x2E, 0x32, 0x58, 0x30, 0x30,
	0x32, 0x00, 0x02, 0x4C, 0x41, 0x4E, 0x4D, 0x41, 0x4E, 0x32, 0x2E, 0x31, 0x00, 0x02, 0x4E, 0x54,
	0x20, 0x4C, 0x4D, 0x20, 0x30, 0x2E, 0x31, 0x32, 0x00,
}
var NegotiateSMBv1Data2 = []byte{
	0x00, 0x00, 0x01, 0x0A, 0xFF, 0x53, 0x4D, 0x42, 0x73, 0x00, 0x00, 0x00, 0x00, 0x18, 0x07, 0xC8,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFE,
	0x00, 0x00, 0x40, 0x00, 0x0C, 0xFF, 0x00, 0x0A, 0x01, 0x04, 0x41, 0x32, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x4A, 0x00, 0x00, 0x00, 0x00, 0x00, 0xD4, 0x00, 0x00, 0xA0, 0xCF, 0x00, 0x60,
	0x48, 0x06, 0x06, 0x2B, 0x06, 0x01, 0x05, 0x05, 0x02, 0xA0, 0x3E, 0x30, 0x3C, 0xA0, 0x0E, 0x30,
	0x0C, 0x06, 0x0A, 0x2B, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0A, 0xA2, 0x2A, 0x04,
	0x28, 0x4E, 0x54, 0x4C, 0x4D, 0x53, 0x53, 0x50, 0x00, 0x01, 0x00, 0x00, 0x00, 0x07, 0x82, 0x08,
	0xA2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x05, 0x02, 0xCE, 0x0E, 0x00, 0x00, 0x00, 0x0F, 0x00, 0x57, 0x00, 0x69, 0x00, 0x6E, 0x00,
	0x64, 0x00, 0x6F, 0x00, 0x77, 0x00, 0x73, 0x00, 0x20, 0x00, 0x53, 0x00, 0x65, 0x00, 0x72, 0x00,
	0x76, 0x00, 0x65, 0x00, 0x72, 0x00, 0x20, 0x00, 0x32, 0x00, 0x30, 0x00, 0x30, 0x00, 0x33, 0x00,
	0x20, 0x00, 0x33, 0x00, 0x37, 0x00, 0x39, 0x00, 0x30, 0x00, 0x20, 0x00, 0x53, 0x00, 0x65, 0x00,
	0x72, 0x00, 0x76, 0x00, 0x69, 0x00, 0x63, 0x00, 0x65, 0x00, 0x20, 0x00, 0x50, 0x00, 0x61, 0x00,
	0x63, 0x00, 0x6B, 0x00, 0x20, 0x00, 0x32, 0x00, 0x00, 0x00, 0x00, 0x00, 0x57, 0x00, 0x69, 0x00,
	0x6E, 0x00, 0x64, 0x00, 0x6F, 0x00, 0x77, 0x00, 0x73, 0x00, 0x20, 0x00, 0x53, 0x00, 0x65, 0x00,
	0x72, 0x00, 0x76, 0x00, 0x65, 0x00, 0x72, 0x00, 0x20, 0x00, 0x32, 0x00, 0x30, 0x00, 0x30, 0x00,
	0x33, 0x00, 0x20, 0x00, 0x35, 0x00, 0x2E, 0x00, 0x32, 0x00, 0x00, 0x00, 0x00, 0x00,
}

var NegotiateSMBv2Data1 = []byte{
	0x00, 0x00, 0x00, 0x45, 0xFF, 0x53, 0x4D, 0x42, 0x72, 0x00,
	0x00, 0x00, 0x00, 0x18, 0x01, 0x48, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF,
	0xAC, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x22, 0x00, 0x02,
	0x4E, 0x54, 0x20, 0x4C, 0x4D, 0x20, 0x30, 0x2E, 0x31, 0x32,
	0x00, 0x02, 0x53, 0x4D, 0x42, 0x20, 0x32, 0x2E, 0x30, 0x30,
	0x32, 0x00, 0x02, 0x53, 0x4D, 0x42, 0x20, 0x32, 0x2E, 0x3F,
	0x3F, 0x3F, 0x00,
}
var NegotiateSMBv2Data2 = []byte{
	0x00, 0x00, 0x00, 0x68, 0xFE, 0x53, 0x4D, 0x42, 0x40, 0x00,
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x24, 0x00,
	0x02, 0x00, 0x01, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x02, 0x02, 0x10, 0x02,
}

func getNTLMSSPNegotiateData(Flags []byte) []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x9A, 0xFE, 0x53, 0x4D, 0x42, 0x40, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x19, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x58, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x60, 0x40, 0x06, 0x06, 0x2B, 0x06, 0x01, 0x05,
		0x05, 0x02, 0xA0, 0x36, 0x30, 0x34, 0xA0, 0x0E, 0x30, 0x0C,
		0x06, 0x0A, 0x2B, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02,
		0x02, 0x0A, 0xA2, 0x22, 0x04, 0x20, 0x4E, 0x54, 0x4C, 0x4D,
		0x53, 0x53, 0x50, 0x00, 0x01, 0x00, 0x00, 0x00,
		Flags[0], Flags[1],
		Flags[2], Flags[3],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}
