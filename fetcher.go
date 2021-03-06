package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
)

type FetcherConfig struct {
	blockService blockservice.BlockService
}

type Fetcher struct {
	// TODO: for now, passing this to instantiation of block session is enough to
	// cancel on session context cancel, but we may want to use this direct reference
	// more tightly in this code
	ctx         context.Context
	blockGetter blockservice.BlockGetter
}

type FetchResult struct {
	Node          ipld.Node
	Path          ipld.Path
	LastBlockPath ipld.Path
	LastBlockLink ipld.Link
}

func NewFetcherConfig(blockService blockservice.BlockService) FetcherConfig {
	return FetcherConfig{blockService: blockService}
}

func (fc FetcherConfig) NewSession(ctx context.Context) *Fetcher {
	return &Fetcher{
		ctx:         ctx,
		blockGetter: blockservice.NewSession(ctx, fc.blockService),
	}
}

func (f *Fetcher) Block(ctx context.Context, c cid.Cid) (ipld.Node, error) {
	nb := basicnode.Prototype.Any.NewBuilder()

	err := cidlink.Link{Cid: c}.Load(ctx, ipld.LinkContext{}, nb, f.loader(ctx))
	if err != nil {
		return nil, err
	}

	return nb.Build(), nil
}

func (f *Fetcher) NodeMatching(ctx context.Context, node ipld.Node, match selector.Selector) (chan FetchResult, chan error) {
	results := make(chan FetchResult)
	errors := make(chan error)

	go func() {
		defer close(results)

		err := f.fetch(ctx, node, match, results)
		if err != nil {
			errors <- err
			return
		}
	}()

	return results, errors
}

func (f *Fetcher) BlockMatching(ctx context.Context, root cid.Cid, match selector.Selector) (chan FetchResult, chan error) {
	results := make(chan FetchResult)
	errors := make(chan error)

	go func() {
		defer close(results)

		// retrieve first node
		node, err := f.Block(ctx, root)
		if err != nil {
			errors <- err
			return
		}

		err = f.fetch(ctx, node, match, results)
		if err != nil {
			errors <- err
			return
		}
	}()

	return results, errors
}

func (f *Fetcher) BlockAll(ctx context.Context, root cid.Cid) (chan FetchResult, chan error) {
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype__Any{})
	allSelector, err := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreUnion(
		ssb.Matcher(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
	)).Selector()
	if err != nil {
		errors := make(chan error, 1)
		errors <- err
		return nil, errors
	}
	return f.BlockMatching(ctx, root, allSelector)
}

func (f *Fetcher) fetch(ctx context.Context, node ipld.Node, match selector.Selector, results chan FetchResult) error {
	return traversal.Progress{
		Cfg: &traversal.Config{
			LinkLoader: f.loader(ctx),
			LinkTargetNodePrototypeChooser: func(_ ipld.Link, _ ipld.LinkContext) (ipld.NodePrototype, error) {
				return basicnode.Prototype__Any{}, nil
			},
		},
	}.WalkMatching(node, match, func(prog traversal.Progress, n ipld.Node) error {
		results <- FetchResult{
			Node:          n,
			Path:          prog.Path,
			LastBlockPath: prog.LastBlock.Path,
			LastBlockLink: prog.LastBlock.Link,
		}
		return nil
	})
}

func (f *Fetcher) loader(ctx context.Context) ipld.Loader {
	return func(lnk ipld.Link, _ ipld.LinkContext) (io.Reader, error) {
		cidLink, ok := lnk.(cidlink.Link)
		if !ok {
			return nil, fmt.Errorf("invalid link type for loading: %v", lnk)
		}

		blk, err := f.blockGetter.GetBlock(ctx, cidLink.Cid)
		if err != nil {
			return nil, err
		}

		return bytes.NewReader(blk.RawData()), nil
	}
}
