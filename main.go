package main

import (
	"flag"
	"log"
	"time"

	"github.com/bitcoinfees/feesim/api"
	"github.com/ziutek/rrd"
)

const (
	step = 60
	coin = 100000000
)

func initRRD(rrdfile string) error {
	c := rrd.NewCreator(rrdfile, time.Now(), step)
	c.DS("fee1", "GAUGE", 3*step, 0, "U")
	c.DS("fee2", "GAUGE", 3*step, 0, "U")
	c.DS("fee3", "GAUGE", 3*step, 0, "U")
	c.DS("fee6", "GAUGE", 3*step, 0, "U")
	c.DS("mempool1", "GAUGE", 3*step, 0, "U")
	c.DS("mempool2", "GAUGE", 3*step, 0, "U")
	c.DS("mempool3", "GAUGE", 3*step, 0, "U")
	c.DS("mempool6", "GAUGE", 3*step, 0, "U")
	c.DS("mempool", "GAUGE", 3*step, 0, "U")
	c.DS("txbyterate", "GAUGE", 3*step, 0, "U")
	c.DS("capbyterate", "GAUGE", 3*step, 0, "U")

	c.RRA("AVERAGE", 0.5, 1, 10080)    // 1 week of 1 min data
	c.RRA("AVERAGE", 0.5, 30, 17520)   // 1 year of 30 min data
	c.RRA("AVERAGE", 0.5, 180, 14600)  // 10 years of 3h data
	c.RRA("AVERAGE", 0.5, 1440, 18250) // 50 years of daily data
	c.RRA("MIN", 0.5, 1440, 18250)
	c.RRA("MAX", 0.5, 1440, 18250)

	return c.Create(false)
}

func main() {
	var (
		rrdfile    string
		host, port string
	)
	flag.StringVar(&rrdfile, "f", "", "Path to RRD file.")
	flag.StringVar(&host, "host", "localhost", "Feesim RPC host address.")
	flag.StringVar(&port, "port", "8350", "Feesim RPC port.")
	flag.Parse()

	if rrdfile == "" {
		log.Fatal("Need to specify RRD file with -f.")
	}

	cfg := api.Config{Host: host, Port: port, Timeout: 15}
	client := api.NewClient(cfg)

	if err := collect(time.Now().Unix(), rrdfile, client); err != nil {
		log.Fatal(err)
	}
}

func collect(t int64, rrdfile string, client *api.Client) error {
	r, err := client.EstimateFee(0)
	if err != nil {
		return err
	}
	state, err := client.MempoolState()
	if err != nil {
		return err
	}
	txrate, err := client.TxRate(0)
	if err != nil {
		return err
	}
	caprate, err := client.CapRate(0)
	if err != nil {
		return err
	}

	result := r.([]interface{})
	mempoolSizeFn := state.SizeFn()
	fees := make([]interface{}, 4)
	mempoolSizes := make([]interface{}, len(fees)+1)
	for i, ridx := range []int{0, 1, 2, 5} {
		r := result[ridx].(float64)
		if r < 0 {
			fees[i] = -1
			mempoolSizes[i] = -1
			continue
		}
		rsats := r * coin
		mempoolSizes[i] = int(mempoolSizeFn.Eval(rsats))
		fees[i] = int(rsats)
	}
	mempoolSizes[len(fees)] = int(mempoolSizeFn.Eval(0))

	capbyterate := caprate["y"][len(caprate["y"])-1]
	txbyterate := txrate["y"][0]

	args := make([]interface{}, 0, 12)
	args = append(args, t)
	args = append(args, fees...)
	args = append(args, mempoolSizes...)
	args = append(args, txbyterate)
	args = append(args, capbyterate)
	log.Println(args...)
	return nil
}
