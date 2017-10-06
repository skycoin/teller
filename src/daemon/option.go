package daemon

// Option session's optional argument type
type Option func(*Session)

func WriteBufferSize(bufsize int) Option {
	return func(s *Session) {
		s.wcBufSize = bufsize
	}
}
