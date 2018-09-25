// +build windows

package api

import "unsafe"

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379284.aspx
const (
	NO_MULTIPLE_TRUSTEE = iota
	TRUSTEE_IS_IMPERSONATE
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379638.aspx
const (
	TRUSTEE_IS_SID = iota
	TRUSTEE_IS_NAME
	TRUSTEE_BAD_FORM
	TRUSTEE_IS_OBJECTS_AND_SID
	TRUSTEE_IS_OBJECTS_AND_NAME
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379639.aspx
const (
	TRUSTEE_IS_UNKNOWN = iota
	TRUSTEE_IS_USER
	TRUSTEE_IS_GROUP
	TRUSTEE_IS_DOMAIN
	TRUSTEE_IS_ALIAS
	TRUSTEE_IS_WELL_KNOWN_GROUP
	TRUSTEE_IS_DELETED
	TRUSTEE_IS_INVALID
	TRUSTEE_IS_COMPUTER
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374899.aspx
const (
	NOT_USED_ACCESS = iota
	GRANT_ACCESS
	SET_ACCESS
	DENY_ACCESS
	REVOKE_ACCESS
	SET_AUDIT_SUCCESS
	SET_AUDIT_FAILURE
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa446627.aspx
const (
	NO_INHERITANCE                     = 0x0
	SUB_OBJECTS_ONLY_INHERIT           = 0x1
	SUB_CONTAINERS_ONLY_INHERIT        = 0x2
	SUB_CONTAINERS_AND_OBJECTS_INHERIT = 0x3
	INHERIT_NO_PROPAGATE               = 0x4
	INHERIT_ONLY                       = 0x8

	OBJECT_INHERIT_ACE       = 0x1
	CONTAINER_INHERIT_ACE    = 0x2
	NO_PROPAGATE_INHERIT_ACE = 0x4
	INHERIT_ONLY_ACE         = 0x8
)

var (
	procSetEntriesInAclW = advapi32.MustFindProc("SetEntriesInAclW")
)

// https://msdn.microsoft.com/ja-jp/library/windows/desktop/aa374931.aspx
type ACL struct {
	ACLRevision byte
	Sbz1        byte
	ACLSize     uint16
	ACECount    uint16
	Sbz2        uint16
	ACEHeader   ACEHeader
}

func (a *ACL) GetACEList() []ACE {
	result := make([]ACE, a.ACECount)
	p := unsafe.Pointer(&a.ACEHeader)
	for i := uint16(0); i < a.ACECount; i++ {
		hCurr := (*ACEHeader)(p)
		switch hCurr.ACEType {
		case ACCESS_ALLOWED_ACE_TYPE:
			result[i] = (*AccessAllowedACE)(p)
		case ACCESS_ALLOWED_CALLBACK_ACE_TYPE:
			result[i] = (*AccessAllowedCallbackACE)(p)
		case ACCESS_ALLOWED_CALLBACK_OBJECT_ACE_TYPE:
			result[i] = (*AccessAllowedCallbackObjectACE)(p)
		case ACCESS_ALLOWED_OBJECT_ACE_TYPE:
			result[i] = (*AccessAllowedObjectACE)(p)
		case ACCESS_DENIED_ACE_TYPE:
			result[i] = (*AccessDeniedACE)(p)
		case ACCESS_DENIED_CALLBACK_ACE_TYPE:
			result[i] = (*AccessDeniedCallbackACE)(p)
		case ACCESS_DENIED_CALLBACK_OBJECT_ACE_TYPE:
			result[i] = (*AccessDeniedCallbackObjectACE)(p)
		case ACCESS_DENIED_OBJECT_ACE_TYPE:
			result[i] = (*AccessDeniedObjectACE)(p)
		case SYSTEM_AUDIT_ACE_TYPE:
			result[i] = (*SystemAuditACE)(p)
		case SYSTEM_AUDIT_CALLBACK_ACE_TYPE:
			result[i] = (*SystemAuditCallbackACE)(p)
		case SYSTEM_AUDIT_CALLBACK_OBJECT_ACE_TYPE:
			result[i] = (*SystemAuditCallbackObjectACE)(p)
		case SYSTEM_AUDIT_OBJECT_ACE_TYPE:
			result[i] = (*SystemAuditCallbackACE)(p)
		case SYSTEM_MANDATORY_LABEL_ACE_TYPE:
			result[i] = (*SystemMandatoryLabelACE)(p)
		}

		p = unsafe.Pointer(uintptr(p) + uintptr(hCurr.ACESize))
	}
	return result
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379636.aspx
type Trustee struct {
	MultipleTrustee          *Trustee
	MultipleTrusteeOperation int32
	TrusteeForm              int32
	TrusteeType              int32
	Name                     *uint16
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa446627.aspx
type ExplicitAccess struct {
	AccessPermissions uint32
	AccessMode        int32
	Inheritance       uint32
	Trustee           Trustee
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379576.aspx
func SetEntriesInAcl(entries []ExplicitAccess, oldAcl *ACL, newAcl **ACL) error {
	ret, _, err := procSetEntriesInAclW.Call(
		uintptr(len(entries)),
		uintptr(unsafe.Pointer(&entries[0])),
		uintptr(unsafe.Pointer(oldAcl)),
		uintptr(unsafe.Pointer(newAcl)),
	)
	if ret != 0 {
		return err
	}
	return nil
}
