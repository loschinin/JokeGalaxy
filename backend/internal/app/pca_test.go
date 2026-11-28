package app

import (
	"context"
	"math"
	"strconv"
	"testing"
)

func TestPCAProjectionUsesLeadingEmbeddingAxis(t *testing.T) {
	vectors := make([][]float32, 4)
	for index := range vectors {
		vectors[index] = make([]float32, embeddingDimensions)
	}
	vectors[0][0], vectors[0][1] = -4, -1
	vectors[1][0], vectors[1][1] = -2, 1
	vectors[2][0], vectors[2][1] = 2, -1
	vectors[3][0], vectors[3][1] = 4, 1

	mean, components, initialized := calculateEmbeddingPCA(vectors)
	if !initialized {
		t.Fatal("calculateEmbeddingPCA() initialized = false, want true")
	}
	for axis := 0; axis < 2; axis++ {
		if len(components[axis]) != embeddingDimensions {
			t.Fatalf("component %d length = %d, want %d", axis, len(components[axis]), embeddingDimensions)
		}
	}
	positive := projectEmbeddingPCA(vectors[3], mean, components)
	negative := projectEmbeddingPCA(vectors[0], mean, components)
	if positive[0] <= 0 || negative[0] >= 0 {
		t.Fatalf("PC1 projections = (%g, %g), want opposite signs", negative[0], positive[0])
	}
	if math.Abs(positive[0]) < 90 || math.Abs(positive[0]) > 110 {
		t.Fatalf("scaled PC1 projection = %g, want approximately 100", positive[0])
	}
}

func TestPCARequiresNondegenerateEmbeddingVariance(t *testing.T) {
	vector := make([]float32, embeddingDimensions)
	vector[0] = 1
	_, _, initialized := calculateEmbeddingPCA([][]float32{vector})
	if initialized {
		t.Fatal("calculateEmbeddingPCA() initialized = true for a single centered embedding")
	}
}

func TestPCAFindsLeadingComponentBeyondLargestMarginalVariance(t *testing.T) {
	vectors := make([][]float32, 4)
	for index := range vectors {
		vectors[index] = make([]float32, embeddingDimensions)
	}
	componentValue := float32(math.Sqrt(5))
	vectors[0][0], vectors[0][1] = componentValue, componentValue
	vectors[1][0], vectors[1][1] = -componentValue, -componentValue
	vectors[2][2], vectors[3][2] = 3, -3

	_, components, initialized := calculateEmbeddingPCA(vectors)
	if !initialized {
		t.Fatal("calculateEmbeddingPCA() initialized = false, want true")
	}
	if math.Abs(components[0][2]) > 1e-6 {
		t.Fatalf("PC1 z loading = %g, want zero despite z having the largest individual variance", components[0][2])
	}
	if math.Abs(math.Abs(components[0][0])-math.Sqrt(.5)) > 1e-5 || math.Abs(math.Abs(components[0][1])-math.Sqrt(.5)) > 1e-5 {
		t.Fatalf("PC1 x/y loadings = (%g, %g), want equal-magnitude loadings", components[0][0], components[0][1])
	}
}

func TestInitializePCABasisReprojectsExistingCoordinates(t *testing.T) {
	jokes := make(map[int64]Joke)
	vectorsByID := make(map[int64][]float32)
	coordinates := make(map[int64][3]float64)
	for id, x := range map[int64]float32{1: -2, 2: -1, 3: 1, 4: 2} {
		vector := make([]float32, embeddingDimensions)
		vector[0] = x
		jokes[id] = Joke{ID: strconv.FormatInt(id, 10), PosX: 99, PosY: 99, PosZ: 99}
		vectorsByID[id] = vector
		coordinates[id] = [3]float64{99, 99, 99}
	}
	server := &Server{jokes: jokes, vectors: vectorsByID, coordinates: coordinates}

	server.initializePCABasis()
	if !server.pcaInitialized {
		t.Fatal("initializePCABasis() left a nondegenerate basis uninitialized")
	}
	if math.Abs(server.jokes[1].PosX+50) > 1e-8 || math.Abs(server.jokes[4].PosX-50) > 1e-8 {
		t.Fatalf("initialized endpoints = (%g, %g), want (-50, 50)", server.jokes[1].PosX, server.jokes[4].PosX)
	}
}

func TestRecompute3DCoordinatesUpdatesJokesAndCoordinateCache(t *testing.T) {
	jokes := make(map[int64]Joke)
	vectorsByID := make(map[int64][]float32)
	coordinates := make(map[int64][3]float64)
	for id, x := range map[int64]float32{1: -2, 2: -1, 3: 1, 4: 2} {
		vector := make([]float32, embeddingDimensions)
		vector[0] = x
		jokes[id] = Joke{ID: strconv.FormatInt(id, 10), PosX: 99, PosY: 99, PosZ: 99}
		vectorsByID[id] = vector
		coordinates[id] = [3]float64{99, 99, 99}
	}
	server := &Server{jokes: jokes, vectors: vectorsByID, coordinates: coordinates}

	count, err := server.recompute3DCoordinates(context.Background())
	if err != nil {
		t.Fatalf("recompute3DCoordinates() error = %v", err)
	}
	if count != 4 {
		t.Fatalf("reprojected count = %d, want 4", count)
	}
	if math.Abs(server.jokes[1].PosX+50) > 1e-8 || math.Abs(server.jokes[4].PosX-50) > 1e-8 {
		t.Fatalf("in-memory endpoints = (%g, %g), want (-50, 50)", server.jokes[1].PosX, server.jokes[4].PosX)
	}
	if server.jokes[1].PosY != 0 || server.jokes[1].PosZ != 0 {
		t.Fatalf("constant axes were not projected to zero: y=%g z=%g", server.jokes[1].PosY, server.jokes[1].PosZ)
	}
	cached := server.coordinates[1]
	if cached != [3]float64{server.jokes[1].PosX, server.jokes[1].PosY, server.jokes[1].PosZ} {
		t.Fatalf("coordinate cache = %v, want joke coordinates", cached)
	}
}
