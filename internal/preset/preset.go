package preset

import (
	"github.com/google/uuid"
)

type Preset struct {
	ID           string
	Name         string
	Description  string
	QueueRating  int
	KeepRating   int
	ReviewRating int
	RejectRating int
}

var (
	DefaultPresets = []*Preset{
		{
			ID:           uuid.New().String(),
			Name:         "草稿阶段筛选",
			Description:  "从大量生成结果中快速筛选",
			QueueRating:  4,
			KeepRating:   4,
			ReviewRating: 0,
			RejectRating: 2,
		},
		{
			ID:           uuid.New().String(),
			Name:         "细化阶段筛选",
			Description:  "从待定图片中精细筛选",
			QueueRating:  0,
			KeepRating:   0,
			ReviewRating: 0,
			RejectRating: 1,
		},
	}
)

type Manager struct {
	presets map[string]*Preset
}

func NewManager() *Manager {
	m := &Manager{
		presets: make(map[string]*Preset),
	}

	for _, preset := range DefaultPresets {
		m.presets[preset.ID] = preset
	}

	return m
}

func (m *Manager) GetAll() []*Preset {
	result := make([]*Preset, 0, len(m.presets))
	for _, preset := range m.presets {
		result = append(result, preset)
	}
	return result
}

func (m *Manager) GetByID(id string) (*Preset, bool) {
	preset, exists := m.presets[id]
	return preset, exists
}

func (m *Manager) Add(preset *Preset) {
	m.presets[preset.ID] = preset
}

func (m *Manager) Delete(id string) {
	delete(m.presets, id)
}
