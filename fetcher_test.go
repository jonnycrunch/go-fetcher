package fetcher_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	testinstance "github.com/ipfs/go-bitswap/testinstance"
	tn "github.com/ipfs/go-bitswap/testnet"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	mockrouting "github.com/ipfs/go-ipfs-routing/mock"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/fluent"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-fetcher"
)

var _ cidlink.MulticodecDecoder = dagcbor.Decoder

func TestFetchIPLDPrimeNode(t *testing.T) {
	block, node, _ := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 3, func(na fluent.MapAssembler) {
		na.AssembleEntry("foo").AssignBool(true)
		na.AssembleEntry("bar").AssignBool(false)
		na.AssembleEntry("nested").CreateMap(2, func(na fluent.MapAssembler) {
			na.AssembleEntry("nonlink").AssignString("zoo")
		})
	}))

	net := tn.VirtualNetwork(mockrouting.NewServer(), delay.Fixed(0*time.Millisecond))
	ig := testinstance.NewTestInstanceGenerator(net, nil, nil)
	defer ig.Close()

	peers := ig.Instances(2)
	hasBlock := peers[0]
	defer hasBlock.Exchange.Close()

	err := hasBlock.Exchange.HasBlock(block)
	require.NoError(t, err)

	wantsBlock := peers[1]
	defer wantsBlock.Exchange.Close()

	wantsGetter := blockservice.New(wantsBlock.Blockstore(), wantsBlock.Exchange)
	fetcherConfig := fetcher.NewFetcherConfig(wantsGetter)
	session := fetcherConfig.NewSession(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	retrievedNode, err := fetcher.Block(ctx, session, cidlink.Link{Cid: block.Cid()})
	require.NoError(t, err)
	assert.Equal(t, node, retrievedNode)
}

func TestFetchIPLDGraph(t *testing.T) {
	block3, node3, link3 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 1, func(na fluent.MapAssembler) {
		na.AssembleEntry("three").AssignBool(true)
	}))
	block4, node4, link4 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 1, func(na fluent.MapAssembler) {
		na.AssembleEntry("four").AssignBool(true)
	}))
	block2, node2, link2 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 2, func(na fluent.MapAssembler) {
		na.AssembleEntry("link3").AssignLink(link3)
		na.AssembleEntry("link4").AssignLink(link4)
	}))
	block1, node1, _ := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 3, func(na fluent.MapAssembler) {
		na.AssembleEntry("foo").AssignBool(true)
		na.AssembleEntry("bar").AssignBool(false)
		na.AssembleEntry("nested").CreateMap(2, func(na fluent.MapAssembler) {
			na.AssembleEntry("link2").AssignLink(link2)
			na.AssembleEntry("nonlink").AssignString("zoo")
		})
	}))

	net := tn.VirtualNetwork(mockrouting.NewServer(), delay.Fixed(0*time.Millisecond))
	ig := testinstance.NewTestInstanceGenerator(net, nil, nil)
	defer ig.Close()

	peers := ig.Instances(2)
	hasBlock := peers[0]
	defer hasBlock.Exchange.Close()

	err := hasBlock.Exchange.HasBlock(block1)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block2)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block3)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block4)
	require.NoError(t, err)

	wantsBlock := peers[1]
	defer wantsBlock.Exchange.Close()

	wantsGetter := blockservice.New(wantsBlock.Blockstore(), wantsBlock.Exchange)
	fetcherConfig := fetcher.NewFetcherConfig(wantsGetter)
	session := fetcherConfig.NewSession(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	nodeCh, errCh := fetcher.BlockAll(ctx, session, cidlink.Link{Cid: block1.Cid()})
	require.NoError(t, err)

	assertNodesInOrder(t, nodeCh, errCh, 10, map[int]ipld.Node{0: node1, 4: node2, 5: node3, 7: node4})
}

func TestHelpers(t *testing.T) {
	block3, node3, link3 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 1, func(na fluent.MapAssembler) {
		na.AssembleEntry("three").AssignBool(true)
	}))
	block4, node4, link4 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 1, func(na fluent.MapAssembler) {
		na.AssembleEntry("four").AssignBool(true)
	}))
	block2, node2, link2 := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 2, func(na fluent.MapAssembler) {
		na.AssembleEntry("link3").AssignLink(link3)
		na.AssembleEntry("link4").AssignLink(link4)
	}))
	block1, node1, _ := encodeBlock(fluent.MustBuildMap(basicnode.Prototype__Map{}, 3, func(na fluent.MapAssembler) {
		na.AssembleEntry("foo").AssignBool(true)
		na.AssembleEntry("bar").AssignBool(false)
		na.AssembleEntry("nested").CreateMap(2, func(na fluent.MapAssembler) {
			na.AssembleEntry("link2").AssignLink(link2)
			na.AssembleEntry("nonlink").AssignString("zoo")
		})
	}))

	net := tn.VirtualNetwork(mockrouting.NewServer(), delay.Fixed(0*time.Millisecond))
	ig := testinstance.NewTestInstanceGenerator(net, nil, nil)
	defer ig.Close()

	peers := ig.Instances(2)
	hasBlock := peers[0]
	defer hasBlock.Exchange.Close()

	err := hasBlock.Exchange.HasBlock(block1)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block2)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block3)
	require.NoError(t, err)
	err = hasBlock.Exchange.HasBlock(block4)
	require.NoError(t, err)

	wantsBlock := peers[1]
	defer wantsBlock.Exchange.Close()

	wantsGetter := blockservice.New(wantsBlock.Blockstore(), wantsBlock.Exchange)

	t.Run("Block retrieves node", func(t *testing.T) {
		fetcherConfig := fetcher.NewFetcherConfig(wantsGetter)
		session := fetcherConfig.NewSession(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		node, err := fetcher.Block(ctx, session, cidlink.Link{Cid: block1.Cid()})
		require.NoError(t, err)

		assert.Equal(t, node, node1)
	})

	t.Run("BlockMatching retrieves nodes matching selector", func(t *testing.T) {
		// limit recursion depth to 2 nodes and expect to get only 2 blocks (4 nodes)
		ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype__Any{})
		sel, err := ssb.ExploreRecursive(selector.RecursionLimitDepth(2), ssb.ExploreUnion(
			ssb.Matcher(),
			ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
		)).Selector()
		require.NoError(t, err)

		fetcherConfig := fetcher.NewFetcherConfig(wantsGetter)
		session := fetcherConfig.NewSession(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		nodeCh, errCh := fetcher.BlockMatching(ctx, session, cidlink.Link{Cid: block1.Cid()}, sel)
		require.NoError(t, err)

		assertNodesInOrder(t, nodeCh, errCh, 4, map[int]ipld.Node{0: node1, 4: node2})
	})

	t.Run("BlockAllOfType retrieves all nodes with a schema", func(t *testing.T) {
		// limit recursion depth to 2 nodes and expect to get only 2 blocks (4 nodes)
		fetcherConfig := fetcher.NewFetcherConfig(wantsGetter)
		session := fetcherConfig.NewSession(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		nodeCh, errCh := fetcher.BlockAllOfType(ctx, session, cidlink.Link{Cid: block1.Cid()}, basicnode.Prototype__Any{})
		require.NoError(t, err)

		assertNodesInOrder(t, nodeCh, errCh, 10, map[int]ipld.Node{0: node1, 4: node2, 5: node3, 7: node4})
	})
}

func assertNodesInOrder(t *testing.T, nodeCh <-chan fetcher.FetchResult, errCh <-chan error, nodeCount int, nodes map[int]ipld.Node) {
	order := 0
Loop:
	for {
		select {
		case res, ok := <-nodeCh:
			if !ok {
				break Loop
			}

			expectedNode, ok := nodes[order]
			if ok {
				assert.Equal(t, expectedNode, res.Node)
			}

			order++
		case err := <-errCh:
			require.FailNow(t, err.Error())
		}
	}
	assert.Equal(t, nodeCount, order)
}

func encodeBlock(n ipld.Node) (blocks.Block, ipld.Node, ipld.Link) {
	lb := cidlink.LinkBuilder{cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   0x17,
		MhLength: 20,
	}}
	var b blocks.Block
	lnk, err := lb.Build(context.Background(), ipld.LinkContext{}, n,
		func(ipld.LinkContext) (io.Writer, ipld.StoreCommitter, error) {
			buf := bytes.Buffer{}
			return &buf, func(lnk ipld.Link) error {
				clnk, ok := lnk.(cidlink.Link)
				if !ok {
					return fmt.Errorf("incorrect link type %v", lnk)
				}
				var err error
				b, err = blocks.NewBlockWithCid(buf.Bytes(), clnk.Cid)
				return err
			}, nil
		},
	)
	if err != nil {
		panic(err)
	}
	return b, n, lnk
}
