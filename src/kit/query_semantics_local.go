package kit

func semanticLocalValue(t typed, v *jv) (*OracleQuerySemantics, *Error) {
	m, e := t.object(v, []string{"phase", "claim", "code", "registration_sha256", "capture", "projection"})
	if e != nil {
		return nil, e
	}
	local := &OracleQuerySemantics{}
	for _, f := range []struct {
		dst  *string
		name string
	}{{&local.Phase, "phase"}, {&local.Claim, "claim"}, {&local.Code, "code"}, {&local.RegistrationSHA256, "registration_sha256"}} {
		if *f.dst, e = t.str(m[f.name]); e != nil {
			return nil, e
		}
	}
	if local.Phase != "PRODUCER_LOCAL" || local.Claim != AssessBlocked || local.Code != "EXTERNAL_APPROVAL_PENDING" || !hex64.MatchString(local.RegistrationSHA256) {
		return nil, t.bad("SEMANTICS_LOCAL_INVALID", v)
	}
	pin := func(v *jv, role string) (ReplayMember, *Error) {
		var p ReplayMember
		m, e := t.object(v, []string{"path", "role", "bytes", "sha256"})
		if e != nil {
			return p, e
		}
		if p.Path, e = t.str(m["path"]); e != nil {
			return p, e
		}
		if p.Role, e = t.str(m["role"]); e != nil {
			return p, e
		}
		if p.Bytes, e = t.uint(m["bytes"]); e != nil {
			return p, e
		}
		if p.SHA256, e = hexDigest(t, m["sha256"]); e != nil {
			return p, e
		}
		if !portable(p.Path, false) || p.Role != role || p.Bytes > MaxDocumentBytes {
			return p, t.bad("SEMANTICS_LOCAL_PIN", v)
		}
		return p, nil
	}
	if local.Projection, e = pin(m["projection"], "projection"); e != nil {
		return nil, e
	}
	if m["capture"].kind != 'n' {
		p, e := pin(m["capture"], "capture")
		if e != nil {
			return nil, e
		}
		local.Capture = &p
	}
	return local, nil
}
