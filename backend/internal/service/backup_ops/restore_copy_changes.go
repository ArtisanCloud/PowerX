package backup_ops

func (s *RestoreCopyService) ChangeDirectories() []string {
	if s == nil || s.initErr != nil || s.store == nil {
		return nil
	}
	return []string{s.store.root}
}
