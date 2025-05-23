package routing

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/lightningnetwork/lnd/zpay32"
	"github.com/stretchr/testify/require"
)

const (
	sourceNodeID = 1
	targetNodeID = 2
)

type mockBandwidthHints struct {
	hints map[uint64]lnwire.MilliSatoshi
}

func (m *mockBandwidthHints) availableChanBandwidth(channelID uint64,
	_ lnwire.MilliSatoshi) (lnwire.MilliSatoshi, bool) {

	if m.hints == nil {
		return 0, false
	}

	balance, ok := m.hints[channelID]
	return balance, ok
}

func (m *mockBandwidthHints) firstHopCustomBlob() fn.Option[tlv.Blob] {
	return fn.None[tlv.Blob]()
}

// integratedRoutingContext defines the context in which integrated routing
// tests run.
type integratedRoutingContext struct {
	graph *mockGraph
	t     *testing.T

	source *mockNode
	target *mockNode

	amt         lnwire.MilliSatoshi
	maxShardAmt *lnwire.MilliSatoshi
	finalExpiry int32

	mcCfg          MissionControlConfig
	pathFindingCfg PathFindingConfig
	routeHints     [][]zpay32.HopHint
}

// newIntegratedRoutingContext instantiates a new integrated routing test
// context with a source and a target node.
func newIntegratedRoutingContext(t *testing.T) *integratedRoutingContext {
	// Instantiate a mock graph.
	source := newMockNode(sourceNodeID)
	target := newMockNode(targetNodeID)

	graph := newMockGraph(t)
	graph.addNode(source)
	graph.addNode(target)
	graph.source = source

	// Initiate the test context with a set of default configuration values.
	// We don't use the lnd defaults here, because otherwise changing the
	// defaults would break the unit tests. The actual values picked aren't
	// critical to excite certain behavior, but do need to be aligned with
	// the test case assertions.
	aCfg := AprioriConfig{
		PenaltyHalfLife:       30 * time.Minute,
		AprioriHopProbability: 0.6,
		AprioriWeight:         0.5,
		CapacityFraction:      testCapacityFraction,
	}
	estimator, err := NewAprioriEstimator(aCfg)
	require.NoError(t, err)

	ctx := integratedRoutingContext{
		t:           t,
		graph:       graph,
		amt:         100000,
		finalExpiry: 40,

		mcCfg: MissionControlConfig{
			Estimator: estimator,
		},

		pathFindingCfg: PathFindingConfig{
			AttemptCost:    1000,
			MinProbability: 0.01,
		},

		source: source,
		target: target,
	}

	return &ctx
}

// htlcAttempt records the route and outcome of an attempted htlc.
type htlcAttempt struct {
	route   *route.Route
	success bool
}

func (h htlcAttempt) String() string {
	return fmt.Sprintf("success=%v, route=%v", h.success, h.route)
}

// testPayment launches a test payment and asserts that it is completed after
// the expected number of attempts.
func (c *integratedRoutingContext) testPayment(maxParts uint32,
	destFeatureBits ...lnwire.FeatureBit) ([]htlcAttempt, error) {

	// We start out with the base set of MPP feature bits. If the caller
	// overrides this set of bits, then we'll use their feature bits
	// entirely.
	baseFeatureBits := mppFeatures
	if len(destFeatureBits) != 0 {
		baseFeatureBits = lnwire.NewRawFeatureVector(destFeatureBits...)
	}

	var (
		nextPid  uint64
		attempts []htlcAttempt
	)

	// Create temporary database for mission control.
	file, err := os.CreateTemp("", "*.db")
	if err != nil {
		c.t.Fatal(err)
	}

	dbPath := file.Name()
	c.t.Cleanup(func() {
		if err := file.Close(); err != nil {
			c.t.Fatal(err)
		}
		if err := os.Remove(dbPath); err != nil {
			c.t.Fatal(err)
		}
	})

	db, err := kvdb.Open(
		kvdb.BoltBackendName, dbPath, true, kvdb.DefaultDBTimeout,
		false,
	)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() {
		if err := db.Close(); err != nil {
			c.t.Fatal(err)
		}
	})

	// Instantiate a new mission controller with the current configuration
	// values.
	mcController, err := NewMissionController(db, c.source.pubkey, &c.mcCfg)
	require.NoError(c.t, err)

	mc, err := mcController.GetNamespacedStore(
		DefaultMissionControlNamespace,
	)
	require.NoError(c.t, err)

	getBandwidthHints := func(_ Graph) (bandwidthHints, error) {
		// Create bandwidth hints based on local channel balances.
		bandwidthHints := map[uint64]lnwire.MilliSatoshi{}
		for _, ch := range c.graph.nodes[c.source.pubkey].channels {
			bandwidthHints[ch.id] = ch.balance
		}

		return &mockBandwidthHints{
			hints: bandwidthHints,
		}, nil
	}

	var paymentAddr [32]byte
	payment := LightningPayment{
		FinalCLTVDelta: uint16(c.finalExpiry),
		FeeLimit:       lnwire.MaxMilliSatoshi,
		Target:         c.target.pubkey,
		PaymentAddr:    fn.Some(paymentAddr),
		DestFeatures: lnwire.NewFeatureVector(
			baseFeatureBits, lnwire.Features,
		),
		Amount:     c.amt,
		CltvLimit:  math.MaxUint32,
		MaxParts:   maxParts,
		RouteHints: c.routeHints,
	}

	var paymentHash [32]byte
	if err := payment.SetPaymentHash(paymentHash); err != nil {
		return nil, err
	}

	if c.maxShardAmt != nil {
		payment.MaxShardAmt = c.maxShardAmt
	}

	session, err := newPaymentSession(
		&payment, c.graph.source.pubkey, getBandwidthHints,
		c.graph, mc, c.pathFindingCfg,
	)
	if err != nil {
		c.t.Fatal(err)
	}

	// Override default minimum shard amount.
	session.minShardAmt = lnwire.NewMSatFromSatoshis(5000)

	// Now the payment control loop starts. It will keep trying routes until
	// the payment succeeds.
	var (
		amtRemaining  = payment.Amount
		inFlightHtlcs uint32
	)
	for {
		// Create bandwidth hints based on local channel balances.
		bandwidthHints := map[uint64]lnwire.MilliSatoshi{}
		for _, ch := range c.graph.nodes[c.source.pubkey].channels {
			bandwidthHints[ch.id] = ch.balance
		}

		// Find a route.
		route, err := session.RequestRoute(
			amtRemaining, lnwire.MaxMilliSatoshi, inFlightHtlcs, 0,
			lnwire.CustomRecords{
				lnwire.MinCustomRecordsTlvType: []byte{1, 2, 3},
			},
		)
		if err != nil {
			return attempts, err
		}

		// Send out the htlc on the mock graph.
		pid := nextPid
		nextPid++
		htlcResult, err := c.graph.sendHtlc(route)
		if err != nil {
			c.t.Fatal(err)
		}

		success := htlcResult.failure == nil
		attempts = append(attempts, htlcAttempt{
			route:   route,
			success: success,
		})

		// Process the result. In normal Lightning operations, the
		// sender doesn't get an acknowledgement from the recipient that
		// the htlc arrived. In integrated routing tests, this
		// acknowledgement is available. It is a simplification of
		// reality that still allows certain classes of tests to be
		// performed.
		if success {
			inFlightHtlcs++

			err := mc.ReportPaymentSuccess(pid, route)
			if err != nil {
				c.t.Fatal(err)
			}

			amtRemaining -= route.ReceiverAmt()

			// If the full amount has been paid, the payment is
			// successful and the control loop can be terminated.
			if amtRemaining == 0 {
				break
			}

			// Otherwise try to send the remaining amount.
			continue
		}

		// Failure, update mission control and retry.
		finalResult, err := mc.ReportPaymentFail(
			pid, route,
			getNodeIndex(route, htlcResult.failureSource),
			htlcResult.failure,
		)
		if err != nil {
			c.t.Fatal(err)
		}

		if finalResult != nil {
			break
		}
	}

	return attempts, nil
}

// getNodeIndex returns the zero-based index of the given node in the route.
func getNodeIndex(route *route.Route, failureSource route.Vertex) *int {
	if failureSource == route.SourcePubKey {
		idx := 0
		return &idx
	}

	for i, h := range route.Hops {
		if h.PubKeyBytes == failureSource {
			idx := i + 1
			return &idx
		}
	}
	return nil
}
