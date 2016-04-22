package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	err := initRRD(rrdfile)
	if err != nil {
		if os.IsExist(err) {
			log.Printf("Existing RRD file found at %s.", rrdfile)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Printf("Creating new RRD file at %s", rrdfile)
	}

	cfg := api.Config{Host: host, Port: port, Timeout: 15}
	client := api.NewClient(cfg)
	done := make(chan struct{})

	// Signal handling
	sigc := make(chan os.Signal, 3)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigc
		close(done)
	}()

	run(rrdfile, client, done)
	log.Println("All done")
}

func run(rrdfile string, client *api.Client, done <-chan struct{}) {
	log.Println("Starting rrd collection..")
	for {
		t := time.Now().Unix()
		nextUpdate := t - t%step + step
		tc := time.After(time.Duration(nextUpdate-t) * time.Second)
		select {
		case <-tc:
		case <-done:
			return
		}
		args, err := collect(nextUpdate, client)
		if err != nil {
			log.Println("[ERROR]", err)
			args = make([]interface{}, 12)
			args[0] = nextUpdate
			for i := range args {
				if i > 0 {
					args[i] = "U"
				}
			}
		}
		u := rrd.NewUpdater(rrdfile)
		if err := u.Update(args...); err != nil {
			log.Println("[ERROR]", err)
		} else {
			log.Println(args)
		}
	}
}

func collect(t int64, client *api.Client) ([]interface{}, error) {
	r, err := client.EstimateFee(0)
	if err != nil {
		return nil, err
	}
	state, err := client.MempoolState()
	if err != nil {
		return nil, err
	}
	txrate, err := client.TxRate(0)
	if err != nil {
		return nil, err
	}
	caprate, err := client.CapRate(0)
	if err != nil {
		return nil, err
	}

	result := r.([]interface{})
	mempoolSizeFn := state.SizeFn()
	fees := make([]interface{}, 4)
	mempoolSizes := make([]interface{}, len(fees)+1)
	for i, ridx := range []int{0, 1, 2, 5} {
		r := result[ridx].(float64)
		if r < 0 {
			fees[i] = "U"
			mempoolSizes[i] = "U"
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
	return args, nil
}
