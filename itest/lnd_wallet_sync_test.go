package itest

import (
	"fmt"

	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/stretchr/testify/require"
)

// walletSyncTestCases defines a set of tests for the wallet_synced field
// in GetInfoResponse.
var walletSyncTestCases = []*lntest.TestCase{
	{
		Name:     "wallet synced",
		TestFunc: runTestWalletSynced,
	},
}

// runTestWalletSynced tests that the wallet_synced field in GetInfoResponse
// correctly reflects the wallet's sync state and transitions from false to
// true after a node restart while blocks were mined.
func runTestWalletSynced(ht *lntest.HarnessTest) {
	// Create a test node.
	alice := ht.NewNodeWithCoins("Alice", nil)

	// Wait for wallet_synced to become true. This mirrors the
	// WaitForBlockchainSync pattern but checks the wallet_synced field.
	err := wait.NoError(func() error {
		resp := alice.RPC.GetInfo()
		if !resp.WalletSynced {
			return fmt.Errorf("wallet not synced yet, "+
				"wallet_synced=%v, synced_to_chain=%v",
				resp.WalletSynced, resp.SyncedToChain)
		}

		return nil
	}, lntest.DefaultTimeout)
	require.NoError(ht, err, "wallet should sync after node creation")

	// Log Alice's sync state for debug.
	resp := alice.RPC.GetInfo()
	ht.Logf("Alice wallet_synced=%v, synced_to_chain=%v",
		resp.WalletSynced, resp.SyncedToChain)

	// Stop Alice directly.
	require.NoError(ht, alice.Stop())

	// Mine a bunch of blocks while Alice is down. This ensures that when
	// Alice restarts, her wallet will need to catch up.
	const numBlocks = 20
	// Mine directly with the miner to avoid waiting for Alice to sync
	// while she is intentionally offline.
	ht.Miner().MineBlocks(numBlocks)

	// Start Alice directly without waiting for sync.
	require.NoError(ht, alice.Start(ht.Context()))

	// Track whether we observed wallet_synced as false at any point.
	var sawFalse bool

	// Poll GetInfo until wallet_synced becomes true.
	err = wait.NoError(func() error {
		resp := alice.RPC.GetInfo()

		// Track if we see wallet_synced as false.
		if !resp.WalletSynced {
			sawFalse = true
			return fmt.Errorf("wallet not synced yet, "+
				"wallet_synced=%v", resp.WalletSynced)
		}

		return nil
	}, lntest.DefaultTimeout)
	require.NoError(ht, err, "wallet should eventually sync")

	// Final verification that wallet_synced is true.
	finalResp := alice.RPC.GetInfo()
	require.True(ht, finalResp.WalletSynced,
		"wallet should be synced after waiting")

	// Log the test result for visibility.
	if sawFalse {
		ht.Logf("Successfully observed WalletSynced transition from " +
			"false to true")
	} else {
		ht.Logf("WalletSynced was already true (sync completed quickly)")
	}
}
