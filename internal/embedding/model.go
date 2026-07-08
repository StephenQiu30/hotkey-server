// Package embedding provides text embedding via an ONNX model loaded in-process.
package embedding

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// Model wraps an ONNX runtime session for text embedding inference.
type Model struct {
	session *ort.AdvancedSession
	inputT  *ort.Tensor[int64]
	maskT   *ort.Tensor[int64]
	outputT *ort.Tensor[float32]
}

// NewModel loads an ONNX embedding model from the given file path.
// The model must have input_ids (int64) and attention_mask (int64) as inputs,
// and produce a 384-dim float32 output (sentence_embedding).
// Call Close when done to release resources.
func NewModel(modelPath string) (*Model, error) {
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnx init: %w", err)
	}

	seqLen := int64(512)
	inputShape := ort.NewShape(1, seqLen)
	inputT, err := ort.NewEmptyTensor[int64](inputShape)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	maskShape := ort.NewShape(1, seqLen)
	maskT, err := ort.NewEmptyTensor[int64](maskShape)
	if err != nil {
		inputT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}

	outputShape := ort.NewShape(1, 384)
	outputT, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputT.Destroy()
		maskT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	// Input/output names depend on how the model was exported to ONNX.
	// bge-small-zh-v1.5 from sentence-transformers exports as:
	//   inputs:  ["input_ids", "attention_mask"]
	//   outputs: ["sentence_embedding"]
	inputNames := []string{"input_ids", "attention_mask"}
	outputNames := []string{"sentence_embedding"}

	session, err := ort.NewAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		[]ort.ArbitraryTensor{inputT, maskT},
		[]ort.ArbitraryTensor{outputT},
		nil,
	)
	if err != nil {
		inputT.Destroy()
		maskT.Destroy()
		outputT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Model{session: session, inputT: inputT, maskT: maskT, outputT: outputT}, nil
}

// Embed runs inference on the given token IDs and returns the embedding vector.
// tokenIDs should include [CLS] and [SEP] markers. Truncated to 512 tokens.
func (m *Model) Embed(tokenIDs []int64) ([384]float32, error) {
	var result [384]float32

	seqLen := len(tokenIDs)
	if seqLen > 512 {
		seqLen = 512
	}

	inputData := make([]int64, 512)
	maskData := make([]int64, 512)
	for i := 0; i < seqLen; i++ {
		inputData[i] = tokenIDs[i]
		maskData[i] = 1
	}

	copy(m.inputT.GetData(), inputData)
	copy(m.maskT.GetData(), maskData)

	if err := m.session.Run(); err != nil {
		return result, fmt.Errorf("onnx run: %w", err)
	}

	copy(result[:], m.outputT.GetData())
	return result, nil
}

// Close releases the ONNX session and environment.
func (m *Model) Close() error {
	var errs []error
	if err := m.session.Destroy(); err != nil {
		errs = append(errs, err)
	}
	m.inputT.Destroy()
	m.maskT.Destroy()
	m.outputT.Destroy()
	ort.DestroyEnvironment()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
