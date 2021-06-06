package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/multivers-simulation/logger"
	"github.com/iotaledger/multivers-simulation/multiverse"
	"github.com/iotaledger/multivers-simulation/network"
)

var log = logger.New("Simulation")
var maxWeight int
var leader multiverse.Color
var oldOpinionAttacker multiverse.Color

const nodesCount = 100

func main() {
	log.Info("Starting simulation ... [DONE]")
	defer log.Info("Shutting down simulation ... [DONE]")

	testNetwork := network.New(
		network.Nodes(nodesCount, multiverse.NewNode, network.ZIPFDistribution(0.8, 100000000)),
		network.Delay(30*time.Millisecond, 250*time.Millisecond),
		network.PacketLoss(0, 0.05),
		network.Topology(network.WattsStrogatz(4, 1)),
	)
	testNetwork.Start()
	defer testNetwork.Shutdown()

	attackers := testNetwork.RandomPeers(10)
	var attacker *network.Peer
	var attackersOpinions = make(map[*network.Peer]multiverse.Color)
	for _, attacker = range attackers {
		attackersOpinions[attacker] = multiverse.UndefinedColor
	}
	monitorNetworkState(testNetwork)
	secureNetwork(testNetwork, attackers, 500*time.Millisecond)

	time.Sleep(2 * time.Second)

	counter := 1
	for {
		for _, attacker = range attackers {

			oldOpinionAttacker = attacker.Node.(*multiverse.Node).Tangle.OpinionManager.Opinion()
			attacker.Node.(*multiverse.Node).Tangle.OpinionManager.Set(multiverse.Color(counter))
			attacker.Node.(*multiverse.Node).Tangle.OpinionManager.Events.OpinionChanged.Trigger(oldOpinionAttacker, multiverse.Color(counter))
			sendMessage(attacker, multiverse.Color(counter))
		}
		counter++
		if counter > 3 { // comment out for increasing number of conflicts
			counter = 1
		}

		time.Sleep(10 * time.Millisecond)
	}
	//time.Sleep(30 * time.Second)
}

var (
	tpsCounter = uint64(0)

	opinions = make(map[multiverse.Color]int)

	opinionMutex sync.Mutex

	relevantValidators int
)

func monitorNetworkState(testNetwork *network.Network) {

	//opinions = make(map[multiverse.Color]int)

	opinions[multiverse.UndefinedColor] = nodesCount
	opinions[multiverse.Blue] = 0
	opinions[multiverse.Red] = 0
	opinions[multiverse.Green] = 0
	for _, peer := range testNetwork.Peers {
		peer.Node.(*multiverse.Node).Tangle.OpinionManager.Events.OpinionChanged.Attach(events.NewClosure(func(oldOpinion multiverse.Color, newOpinion multiverse.Color) {
			opinionMutex.Lock()
			defer opinionMutex.Unlock()
			//if _, ok := opinions[newOpinion]; ok {
			opinions[oldOpinion]--
			opinions[newOpinion]++
			//} else {
			//	opinions[oldOpinion]--
			//	opinions[newOpinion] = 1
			//}

		}))
	}

	go func() {
		for range time.Tick(1000 * time.Millisecond) {
			maxWeight = 0
			leader = multiverse.UndefinedColor
			for k := range opinions {
				if opinions[k] > maxWeight {
					maxWeight = opinions[k]
					leader = k
				}
			}
			log.Infof("Network Status: %d TPS :: Leader %d  : Votes for Leader %d  :: %d Nodes :: %d Validators",
				atomic.LoadUint64(&tpsCounter),
				leader,
				maxWeight,
				nodesCount,
				relevantValidators,
			)
			//log.Infof("Opinion of node 1:", testNetwork.Peers[1].Node.(*multiverse.Node).Tangle.OpinionManager.Opinion())
			//log.Infof("Opinions:", opinions)
			atomic.StoreUint64(&tpsCounter, 0)
		}
	}()
}

func secureNetwork(testNetwork *network.Network, attackers []*network.Peer, pace time.Duration) {
	largestWeight := float64(testNetwork.WeightDistribution.LargestWeight())

	for _, peer := range testNetwork.Peers {
		if !find(attackers, peer) {
			continue
		}
		weightOfPeer := float64(testNetwork.WeightDistribution.Weight(peer.ID))
		if 1000*weightOfPeer <= largestWeight {
			continue
		}

		relevantValidators++

		go startSecurityWorker(peer, time.Duration(largestWeight/weightOfPeer*float64(pace/time.Millisecond))*time.Millisecond)
	}
}

func startSecurityWorker(peer *network.Peer, pace time.Duration) {
	for range time.Tick(pace) {
		sendMessage(peer)
	}
}

func sendMessage(peer *network.Peer, optionalColor ...multiverse.Color) {
	atomic.AddUint64(&tpsCounter, 1)

	if len(optionalColor) >= 1 {
		peer.Node.(*multiverse.Node).IssuePayload(optionalColor[0])
	} else {
		peer.Node.(*multiverse.Node).IssuePayload(multiverse.UndefinedColor)
	}

}

func find(source []*network.Peer, value *network.Peer) bool {
	for _, item := range source {
		if item == value {
			return true
		}
	}
	return false
}
