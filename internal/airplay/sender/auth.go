package sender

// PasswordRejected reports whether the receiver refused the saved password
// during this session's handshake.
func (s *Sender) PasswordRejected() bool { return s.passwordRejected }
