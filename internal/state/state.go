package state

import (
	"fmt"
	"os"
	"time"

	"github.com/gofrs/flock"

	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/storage"
)

func LoadCharacterNote(root, project string) (*model.CharacterNote, error) {
	if err := storage.ValidateName(project); err != nil {
		return nil, err
	}
	note := &model.CharacterNote{}
	if err := storage.LoadJSON(storage.CharactersPath(root, project), note); err != nil {
		return nil, fmt.Errorf("loading character note: %w", err)
	}
	return note, nil
}

func SaveCharacterNote(root, project string, note *model.CharacterNote) error {
	if err := storage.ValidateName(project); err != nil {
		return err
	}
	if err := storage.EnsureDir(storage.StateDir(root, project)); err != nil {
		return fmt.Errorf("ensuring state dir: %w", err)
	}
	if err := storage.SaveJSON(storage.CharactersPath(root, project), note); err != nil {
		return fmt.Errorf("saving character note: %w", err)
	}
	return nil
}

func LoadSceneList(root, project string) (*model.SceneList, error) {
	if err := storage.ValidateName(project); err != nil {
		return nil, err
	}
	scenes := &model.SceneList{}
	if err := storage.LoadJSON(storage.ScenesPath(root, project), scenes); err != nil {
		return nil, fmt.Errorf("loading scene list: %w", err)
	}
	return scenes, nil
}

func SaveSceneList(root, project string, scenes *model.SceneList) error {
	if err := storage.ValidateName(project); err != nil {
		return err
	}
	if err := storage.EnsureDir(storage.StateDir(root, project)); err != nil {
		return fmt.Errorf("ensuring state dir: %w", err)
	}
	if err := storage.SaveJSON(storage.ScenesPath(root, project), scenes); err != nil {
		return fmt.Errorf("saving scene list: %w", err)
	}
	return nil
}

func LoadManifest(root, project string) (*model.ProjectManifest, error) {
	if err := storage.ValidateName(project); err != nil {
		return nil, err
	}
	m := &model.ProjectManifest{}
	if err := storage.LoadYAML(storage.ManifestPath(root, project), m); err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}
	return m, nil
}

func SaveManifest(root, project string, m *model.ProjectManifest) error {
	if err := storage.ValidateName(project); err != nil {
		return err
	}
	if err := storage.EnsureDir(storage.ProjectDir(root, project)); err != nil {
		return fmt.Errorf("ensuring project dir: %w", err)
	}
	if err := storage.SaveYAML(storage.ManifestPath(root, project), m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}
	return nil
}

func ManifestExists(root, project string) (bool, error) {
	if err := storage.ValidateName(project); err != nil {
		return false, err
	}
	_, err := os.Stat(storage.ManifestPath(root, project))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking manifest: %w", err)
	}
	return true, nil
}

func withManifestLock(root, project string, fn func() error) error {
	lockPath := storage.ManifestLockPath(root, project)
	if err := storage.EnsureDir(storage.ProjectDir(root, project)); err != nil {
		return fmt.Errorf("ensuring project dir for lock: %w", err)
	}
	fileLock := flock.New(lockPath)
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("acquiring manifest lock: %w", err)
	}
	defer fileLock.Unlock()
	return fn()
}

func UpdateManifestPhase(root, project, phase string, status model.PhaseStatus) error {
	return withManifestLock(root, project, func() error {
		m, err := LoadManifest(root, project)
		if err != nil {
			return fmt.Errorf("loading manifest for update: %w", err)
		}

		m.Pipeline.Phases[phase] = status

		allDone := true
		anyFailed := false
		currentPhase := 0
		for _, p := range model.AllPhases {
			ps, ok := m.Pipeline.Phases[p]
			if !ok {
				continue
			}
			if ps.Status == model.PhaseFailed {
				anyFailed = true
				break
			}
			if ps.Status == model.PhaseCompleted {
				if num, ok := model.PhaseNumbers[p]; ok && num > currentPhase {
					currentPhase = num
				}
				continue
			}
			allDone = false
			break
		}

		switch {
		case anyFailed:
			m.Pipeline.Status = model.PhaseFailed
		case allDone:
			m.Pipeline.Status = model.PhaseCompleted
		default:
			m.Pipeline.Status = model.PhaseInProgress
		}
		m.Pipeline.CurrentPhase = currentPhase

		return SaveManifest(root, project, m)
	})
}

func RecordPhaseError(root, project, phase string, err error) error {
	return withManifestLock(root, project, func() error {
		m, loadErr := LoadManifest(root, project)
		if loadErr != nil {
			return loadErr
		}

		m.Pipeline.Errors = append(m.Pipeline.Errors, model.PhaseError{
			Phase:       phase,
			Timestamp:   time.Now().UTC(),
			Message:     err.Error(),
			Recoverable: false,
		})
		phaseStatus := m.Pipeline.Phases[phase]
		phaseStatus.Status = model.PhaseFailed
		phaseStatus.CompletedAt = nil
		m.Pipeline.Phases[phase] = phaseStatus
		m.Pipeline.Status = model.PhaseFailed
		if num, ok := model.PhaseNumbers[phase]; ok {
			m.Pipeline.CurrentPhase = num
		}

		return SaveManifest(root, project, m)
	})
}
