package screenshot

type Command interface {
	Apply(doc *Document) error
	Undo(doc *Document) error
	Name() string
}

type CommandStack struct {
	undo []Command
	redo []Command
}

func (s *CommandStack) Push(command Command) {
	if command == nil {
		return
	}

	s.undo = append(s.undo, command)
	s.redo = nil
}

func (s *CommandStack) CanUndo() bool {
	return len(s.undo) > 0
}

func (s *CommandStack) CanRedo() bool {
	return len(s.redo) > 0
}
