package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
)
var (
	opsQueued = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "run_times",
			Help:      "Number of run times.",
		},
		[]string{
			// Which user has requested the operation?
			"addr",
			// Of what type is the operation?
			"type",
		},
	)
	pingResult = prometheus.NewGauge(
		prometheus.GaugeOpts{
		Name: "ping_avg_time",
		Help: "Time of the ping avg.",
		})
	pingLost = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ping_lost",
			Help: "lost of the ping.",
		})
)
func init() {
	// Metrics have to be registered to be exposed:
	prometheus.MustRegister(opsQueued)
	prometheus.MustRegister(pingResult)
	prometheus.MustRegister(pingLost)
}
const (
	MAX_PG = 2000
)

// 封装 icmp 报头
type ICMP struct {
	Type        uint8
	Code        uint8
	Checksum    uint16
	Identifier  uint16
	SequenceNum uint16
}

var (
	originBytes []byte
	githubPath string = "https://github.com"
	version string = "v1.0"
	listenAddress string
	help bool
	pingAddress string
	pingTimes int64
)

func init() {
	originBytes = make([]byte, MAX_PG)
	flag.StringVar(&listenAddress,"port","8888","this exporter listened port")
	flag.BoolVar(&help,"h",false,"help")
	flag.Int64Var(&pingTimes,"count",4,"ping count")
	flag.StringVar(&pingAddress,"pingaddr","www.baidu.com","pingaddr")
}

func CheckSum(data []byte) (rt uint16) {
	var (
		sum    uint32
		length int = len(data)
		index  int
	)
	for length > 1 {
		sum += uint32(data[index])<<8 + uint32(data[index+1])
		index += 2
		length -= 2
	}
	if length > 0 {
		sum += uint32(data[index]) << 8
	}
	rt = uint16(sum) + uint16(sum>>16)

	return ^rt
}

func Ping(domain string, PS, Count int) {
	var (
		icmp                      ICMP
		laddr                     = net.IPAddr{IP: net.ParseIP("0.0.0.0")} // 得到本机的IP地址结构
		raddr, _                  = net.ResolveIPAddr("ip", domain)        // 解析域名得到 IP 地址结构
		max_lan, min_lan, avg_lan float64
	)
	// 返回一个 ip socket
	conn, err := net.DialIP("ip4:icmp", &laddr, raddr)

	if err != nil {
		fmt.Println(`socket Error:`+err.Error())
		pingResult.Set(3000)
		pingLost.Set(100)
		return
	}

	defer conn.Close()

	// 初始化 icmp 报文
	icmp = ICMP{8, 0, 0, 0, 0}

	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, icmp)
	//fmt.Println(buffer.Bytes())
	binary.Write(&buffer, binary.BigEndian, originBytes[0:PS])
	b := buffer.Bytes()
	binary.BigEndian.PutUint16(b[2:], CheckSum(b))

	//fmt.Println(b)
	fmt.Printf("\n正在 Ping %s 具有 %d(%d) 字节的数据:\n", raddr.String(), PS, PS+28)
	recv := make([]byte, 1024)
	ret_list := []float64{}

	dropPack := 0.0 /*统计丢包的次数，用于计算丢包率*/
	max_lan = 3000.0
	min_lan = 0.0
	avg_lan = 0.0

	for i := Count; i > 0; i-- {
		/*
			向目标地址发送二进制报文包
			如果发送失败就丢包 ++
		*/
		if _, err := conn.Write(buffer.Bytes()); err != nil {
			dropPack++
			time.Sleep(time.Second)
			continue
		}
		// 否则记录当前得时间
		t_start := time.Now()
		conn.SetReadDeadline((time.Now().Add(time.Second * 3)))
		len, err := conn.Read(recv)
		/*
			查目标地址是否返回失败
			如果返回失败则丢包 ++
		*/
		if err != nil {
			dropPack++
			time.Sleep(time.Second)
			continue
		}
		t_end := time.Now()
		dur := float64(t_end.Sub(t_start).Nanoseconds()) / 1e6
		ret_list = append(ret_list, dur)
		if dur < max_lan {
			max_lan = dur
		}
		if dur > min_lan {
			min_lan = dur
		}
		fmt.Printf("来自 %s 的回复: 大小 = %d byte 时间 = %.3fms\n", raddr.String(), len ,dur)
		time.Sleep(time.Second)
	}
	fmt.Printf("丢包率: %.2f%%\n", dropPack/float64(Count)*100)
	if len(ret_list) == 0 {
		avg_lan = 3000.0
	} else {
		sum := 0.0
		for _, n := range ret_list {
			sum += n
		}
		avg_lan = sum / float64(len(ret_list))
	}
	fmt.Printf("rtt 最短 = %.3fms 平均 = %.3fms 最长 = %.3fms\n", min_lan, avg_lan, max_lan)
	pingResult.Set(Decimal(avg_lan))
	pingLost.Set(Decimal(dropPack/float64(Count)*100))
}

func Decimal(value float64) float64 {
	value, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return value
}

func main() {
	flag.Parse()
	if help {
		flag.Usage()
		return
	}
	if(pingTimes>100){
		fmt.Printf("最大次数不能超过100")
		return
	}
	go func() {
		for{
			fmt.Printf("开始执行ping \n")
			Ping(pingAddress, 32, int(pingTimes))
			opsQueued.WithLabelValues(pingAddress, "ping").Inc()
			time.Sleep(10 * time.Second)
		}
	}()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Ping_exporter</title></head>
             <body>
             <h1><a style="text-decoration:none" href='` + githubPath + `'>Ping_exporter</a></h1>
             <p><a href='\metrics'>Metrics</a></p>
             <h2>Build</h2>
             <pre>` + version + `</pre>
             </body>
             </html>`))
	})
	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(`:`+listenAddress, nil))

}
