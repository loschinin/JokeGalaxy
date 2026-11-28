package app

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"
)

const pcaCoordinateScale = 25.0
const pcaMaxIterations = 512

func embeddingText(text string, triggers []string) string {
	return "Темы: " + strings.Join(triggers, ", ") + ". " + text
}

func isUsableEmbedding(values []float32) bool {
	if len(values) != embeddingDimensions {
		return false
	}
	var norm float64
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		norm += float64(value) * float64(value)
	}
	return norm > 1e-12
}

func isLegacyHashEmbedding(joke Joke) bool {
	if len(joke.Embedding) != embeddingDimensions*4 {
		return false
	}
	legacy := embeddingBytes(hashEmbedding(embeddingText(joke.Text, joke.Triggers)))
	return bytes.Equal(joke.Embedding, legacy)
}

func storedSemanticVector(joke Joke) ([]float32, bool) {
	if isLegacyHashEmbedding(joke) {
		return nil, false
	}
	vector := vectorFromBytes(joke.Embedding)
	return vector, isUsableEmbedding(vector)
}

func semanticEmbeddingBytes(values []float32, err error) []byte {
	if err != nil || !isUsableEmbedding(values) {
		return nil
	}
	return embeddingBytes(values)
}

type pcaRecord struct {
	id       int64
	vector   []float32
	position [3]float64
}

// calculateEmbeddingPCA computes the mean and the leading three eigenvectors of
// the embedding covariance matrix. It uses a dense covariance matrix and
// deterministic power iteration with rank-one deflation, so no cluster labels
// or external numerical libraries affect the projection.
func calculateEmbeddingPCA(vectors [][]float32) ([]float64, [3][]float64, bool) {
	mean := make([]float64, embeddingDimensions)
	components := canonicalPCAComponents()
	validCount := 0
	for _, vector := range vectors {
		if !isUsableEmbedding(vector) {
			continue
		}
		validCount++
		for dimension, value := range vector {
			mean[dimension] += float64(value)
		}
	}
	if validCount == 0 {
		return mean, components, false
	}
	for dimension := range mean {
		mean[dimension] /= float64(validCount)
	}

	covariance := make([]float64, embeddingDimensions*embeddingDimensions)
	centered := make([]float64, embeddingDimensions)
	for _, vector := range vectors {
		if !isUsableEmbedding(vector) {
			continue
		}
		for dimension, value := range vector {
			centered[dimension] = float64(value) - mean[dimension]
		}
		for row := 0; row < embeddingDimensions; row++ {
			rowValue := centered[row]
			rowOffset := row * embeddingDimensions
			for column := row; column < embeddingDimensions; column++ {
				covariance[rowOffset+column] += rowValue * centered[column]
			}
		}
	}
	inverseCount := 1 / float64(validCount)
	trace := 0.0
	for row := 0; row < embeddingDimensions; row++ {
		rowOffset := row * embeddingDimensions
		for column := row; column < embeddingDimensions; column++ {
			value := covariance[rowOffset+column] * inverseCount
			covariance[rowOffset+column] = value
			covariance[column*embeddingDimensions+row] = value
		}
		trace += covariance[rowOffset+row]
	}

	if trace <= 0 || math.IsNaN(trace) || math.IsInf(trace, 0) {
		return mean, components, false
	}
	tolerance := trace * 1e-10
	var eigenvalues [3]float64
	for axis := range components {
		vector, eigenvalue := dominantResidualComponent(covariance, components, eigenvalues, axis, tolerance)
		if vector == nil {
			components[axis] = canonicalOrthogonalComponent(components, axis)
			continue
		}
		components[axis] = vector
		eigenvalues[axis] = eigenvalue
	}
	return mean, components, true
}

func canonicalPCAComponents() [3][]float64 {
	var components [3][]float64
	for axis := range components {
		components[axis] = make([]float64, embeddingDimensions)
		components[axis][axis] = 1
	}
	return components
}

func dominantResidualAxis(covariance []float64, components [3][]float64, eigenvalues [3]float64, count int, tolerance float64) (int, float64) {
	bestIndex := -1
	bestVariance := tolerance
	for dimension := 0; dimension < embeddingDimensions; dimension++ {
		variance := covariance[dimension*embeddingDimensions+dimension]
		for axis := 0; axis < count; axis++ {
			loading := components[axis][dimension]
			variance -= eigenvalues[axis] * loading * loading
		}
		if variance > bestVariance {
			bestIndex = dimension
			bestVariance = variance
		}
	}
	return bestIndex, bestVariance
}

func dominantResidualComponent(covariance []float64, components [3][]float64, eigenvalues [3]float64, count int, tolerance float64) ([]float64, float64) {
	start, variance := dominantResidualAxis(covariance, components, eigenvalues, count, tolerance)
	if start < 0 || variance <= tolerance {
		return nil, 0
	}

	bestEigenvalue := tolerance
	var bestComponent []float64
	for attempt := 0; attempt <= 4; attempt++ {
		vector := make([]float64, embeddingDimensions)
		if attempt == 0 {
			vector[start] = 1
		} else {
			fillDeterministicPCAStart(vector, count, attempt)
		}
		orthogonalizePCA(vector, components, count)
		if !normalizePCA(vector) {
			continue
		}

		next := make([]float64, embeddingDimensions)
		for iteration := 0; iteration < pcaMaxIterations; iteration++ {
			applyDeflatedCovariance(covariance, vector, components, eigenvalues, count, next)
			orthogonalizePCA(next, components, count)
			if !normalizePCA(next) {
				break
			}
			similarity := dotPCA(vector, next)
			if similarity < 0 {
				for dimension := range next {
					next[dimension] = -next[dimension]
				}
				similarity = -similarity
			}
			vector, next = next, vector
			if similarity >= 1-1e-10 {
				break
			}
		}

		scratch := make([]float64, embeddingDimensions)
		applyDeflatedCovariance(covariance, vector, components, eigenvalues, count, scratch)
		eigenvalue := dotPCA(vector, scratch)
		if eigenvalue <= bestEigenvalue {
			continue
		}
		canonicalizePCASign(vector)
		bestComponent = append([]float64(nil), vector...)
		bestEigenvalue = eigenvalue
	}
	return bestComponent, bestEigenvalue
}

func fillDeterministicPCAStart(vector []float64, axis, attempt int) {
	state := uint64(0x243f6a8885a308d3) ^ uint64(axis+1)*0x9e3779b97f4a7c15 ^ uint64(attempt)*0xbf58476d1ce4e5b9
	for dimension := range vector {
		state += 0x9e3779b97f4a7c15
		value := state
		value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
		value = (value ^ (value >> 27)) * 0x94d049bb133111eb
		value ^= value >> 31
		unit := float64(value>>11) / 9007199254740992.0
		vector[dimension] = 2*unit - 1
	}
}

func canonicalizePCASign(vector []float64) {
	for _, value := range vector {
		if math.Abs(value) <= 1e-12 {
			continue
		}
		if value < 0 {
			for dimension := range vector {
				vector[dimension] = -vector[dimension]
			}
		}
		return
	}
}

func applyDeflatedCovariance(covariance, vector []float64, components [3][]float64, eigenvalues [3]float64, count int, output []float64) {
	for row := 0; row < embeddingDimensions; row++ {
		rowOffset := row * embeddingDimensions
		sum := 0.0
		for column := 0; column < embeddingDimensions; column++ {
			sum += covariance[rowOffset+column] * vector[column]
		}
		output[row] = sum
	}
	for axis := 0; axis < count; axis++ {
		projection := dotPCA(components[axis], vector)
		factor := eigenvalues[axis] * projection
		for dimension := 0; dimension < embeddingDimensions; dimension++ {
			output[dimension] -= factor * components[axis][dimension]
		}
	}
}

func canonicalOrthogonalComponent(components [3][]float64, count int) []float64 {
	for candidate := 0; candidate < embeddingDimensions; candidate++ {
		vector := make([]float64, embeddingDimensions)
		vector[candidate] = 1
		orthogonalizePCA(vector, components, count)
		if normalizePCA(vector) {
			return vector
		}
	}
	return make([]float64, embeddingDimensions)
}

func orthogonalizePCA(vector []float64, components [3][]float64, count int) {
	for pass := 0; pass < 2; pass++ {
		for axis := 0; axis < count; axis++ {
			projection := dotPCA(vector, components[axis])
			for dimension := 0; dimension < embeddingDimensions; dimension++ {
				vector[dimension] -= projection * components[axis][dimension]
			}
		}
	}
}

func normalizePCA(vector []float64) bool {
	normSquared := dotPCA(vector, vector)
	if normSquared <= 0 || math.IsNaN(normSquared) || math.IsInf(normSquared, 0) {
		return false
	}
	inverseNorm := 1 / math.Sqrt(normSquared)
	for dimension := range vector {
		vector[dimension] *= inverseNorm
	}
	return true
}

func dotPCA(left, right []float64) float64 {
	if len(left) != embeddingDimensions || len(right) != embeddingDimensions {
		return 0
	}
	sum := 0.0
	for dimension := 0; dimension < embeddingDimensions; dimension++ {
		sum += left[dimension] * right[dimension]
	}
	return sum
}

func projectEmbeddingPCA(vector []float32, mean []float64, components [3][]float64) [3]float64 {
	var position [3]float64
	if !isUsableEmbedding(vector) || len(mean) != embeddingDimensions {
		return position
	}
	for axis := range position {
		if len(components[axis]) != embeddingDimensions {
			continue
		}
		projection := 0.0
		for dimension, value := range vector {
			projection += (float64(value) - mean[dimension]) * components[axis][dimension]
		}
		position[axis] = projection * pcaCoordinateScale
	}
	return position
}

func (s *Server) pcaRecordsSnapshot() ([]pcaRecord, uint64) {
	s.mu.RLock()
	records := make([]pcaRecord, 0, len(s.jokes))
	for id := range s.jokes {
		record := pcaRecord{id: id}
		if vector := s.vectors[id]; isUsableEmbedding(vector) {
			record.vector = append([]float32(nil), vector...)
		}
		records = append(records, record)
	}
	version := s.pcaDataVersion
	s.mu.RUnlock()
	sort.Slice(records, func(left, right int) bool { return records[left].id < records[right].id })
	return records, version
}

func pcaVectors(records []pcaRecord) [][]float32 {
	vectors := make([][]float32, 0, len(records))
	for _, record := range records {
		if len(record.vector) == embeddingDimensions {
			vectors = append(vectors, record.vector)
		}
	}
	return vectors
}

// initializePCABasis lazily initializes the projection axes for single-joke
// creation. Once initialized, adding a joke only requires three dot products.
func (s *Server) initializePCABasis() {
	s.pcaMu.Lock()
	defer s.pcaMu.Unlock()
	for attempt := 0; attempt < 3; attempt++ {
		s.mu.RLock()
		if s.pcaInitialized {
			s.mu.RUnlock()
			return
		}
		s.mu.RUnlock()

		records, version := s.pcaRecordsSnapshot()
		mean, components, initialized := calculateEmbeddingPCA(pcaVectors(records))
		if initialized {
			for index := range records {
				if len(records[index].vector) == embeddingDimensions {
					records[index].position = projectEmbeddingPCA(records[index].vector, mean, components)
				}
			}
			persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			err := s.persistPCACoordinates(persistCtx, records)
			cancel()
			if err != nil {
				log.Printf("3D PCA basis initialization could not persist coordinates: %v", err)
				return
			}
		}
		s.mu.Lock()
		if s.pcaInitialized {
			s.mu.Unlock()
			return
		}
		if s.pcaDataVersion != version {
			s.mu.Unlock()
			continue
		}
		s.pcaMean = mean
		s.pcaComponents = components
		s.pcaInitialized = initialized
		if initialized {
			for _, record := range records {
				joke, ok := s.jokes[record.id]
				if !ok {
					continue
				}
				joke.PosX, joke.PosY, joke.PosZ = record.position[0], record.position[1], record.position[2]
				s.jokes[record.id] = joke
				s.coordinates[record.id] = record.position
			}
		}
		s.mu.Unlock()
		return
	}
}

func (s *Server) placeCoordinates(vector []float32) [3]float64 {
	if !isUsableEmbedding(vector) {
		return [3]float64{}
	}
	s.mu.RLock()
	initialized := s.pcaInitialized
	s.mu.RUnlock()
	if !initialized {
		s.initializePCABasis()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.pcaInitialized {
		return [3]float64{}
	}
	return projectEmbeddingPCA(vector, s.pcaMean, s.pcaComponents)
}

func (s *Server) needs3DRecompute() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.jokes) == 0 {
		return false
	}
	if !s.pcaInitialized {
		return true
	}
	if len(s.jokes) < 2 {
		return true
	}

	minimum := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	maximum := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for id := range s.jokes {
		position, ok := s.coordinates[id]
		if !ok {
			return true
		}
		for axis, value := range position {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return true
			}
			minimum[axis] = math.Min(minimum[axis], value)
			maximum[axis] = math.Max(maximum[axis], value)
		}
	}
	for axis := range minimum {
		if maximum[axis]-minimum[axis] > 1e-6 {
			return false
		}
	}
	return true
}

func (s *Server) recompute3DCoordinates(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.pcaMu.Lock()
	defer s.pcaMu.Unlock()

	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		records, version := s.pcaRecordsSnapshot()
		mean, components, initialized := calculateEmbeddingPCA(pcaVectors(records))
		validEmbeddings := 0
		for index := range records {
			if len(records[index].vector) == embeddingDimensions {
				records[index].position = projectEmbeddingPCA(records[index].vector, mean, components)
				validEmbeddings++
			}
		}
		if err := s.persistPCACoordinates(ctx, records); err != nil {
			return 0, err
		}

		s.mu.Lock()
		if s.pcaDataVersion != version {
			s.mu.Unlock()
			continue
		}
		s.pcaMean = mean
		s.pcaComponents = components
		s.pcaInitialized = initialized
		for _, record := range records {
			joke, ok := s.jokes[record.id]
			if !ok {
				continue
			}
			joke.PosX, joke.PosY, joke.PosZ = record.position[0], record.position[1], record.position[2]
			s.jokes[record.id] = joke
			s.coordinates[record.id] = record.position
		}
		s.mu.Unlock()

		log.Printf("3D PCA recompute: re-projected %d semantic embeddings; reset %d jokes without usable embeddings", validEmbeddings, len(records)-validEmbeddings)
		return validEmbeddings, nil
	}
	return 0, fmt.Errorf("joke embeddings changed during PCA recomputation")
}

func (s *Server) persistPCACoordinates(ctx context.Context, records []pcaRecord) error {
	if s.db == nil || len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `UPDATE jokes SET pos_x=$1,pos_y=$2,pos_z=$3 WHERE id=$4`)
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, err := statement.ExecContext(ctx, record.position[0], record.position[1], record.position[2], record.id); err != nil {
			_ = statement.Close()
			return err
		}
	}
	if err := statement.Close(); err != nil {
		return err
	}
	return tx.Commit()
}
