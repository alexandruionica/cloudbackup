// +build windows

package api

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374919.aspx
const (
	ACCESS_MIN_MS_ACE_TYPE    = 0x0
	ACCESS_ALLOWED_ACE_TYPE   = 0x0
	ACCESS_DENIED_ACE_TYPE    = 0x1
	SYSTEM_AUDIT_ACE_TYPE     = 0x2
	SYSTEM_ALARM_ACE_TYPE     = 0x3
	ACCESS_MAX_MS_V2_ACE_TYPE = 0x3

	ACCESS_ALLOWED_COMPOUND_ACE_TYPE = 0x4
	ACCESS_MAX_MS_V3_ACE_TYPE        = 0x4

	ACCESS_MIN_MS_OBJECT_ACE_TYPE  = 0x5
	ACCESS_ALLOWED_OBJECT_ACE_TYPE = 0x5
	ACCESS_DENIED_OBJECT_ACE_TYPE  = 0x6
	SYSTEM_AUDIT_OBJECT_ACE_TYPE   = 0x7
	SYSTEM_ALARM_OBJECT_ACE_TYPE   = 0x8
	ACCESS_MAX_MS_OBJECT_ACE_TYPE  = 0x8

	ACCESS_MAX_MS_V4_ACE_TYPE = 0x8
	ACCESS_MAX_MS_ACE_TYPE    = 0x8

	ACCESS_ALLOWED_CALLBACK_ACE_TYPE        = 0x9
	ACCESS_DENIED_CALLBACK_ACE_TYPE         = 0xA
	ACCESS_ALLOWED_CALLBACK_OBJECT_ACE_TYPE = 0xB
	ACCESS_DENIED_CALLBACK_OBJECT_ACE_TYPE  = 0xC
	SYSTEM_AUDIT_CALLBACK_ACE_TYPE          = 0xD
	SYSTEM_ALARM_CALLBACK_ACE_TYPE          = 0xE
	SYSTEM_AUDIT_CALLBACK_OBJECT_ACE_TYPE   = 0xF
	SYSTEM_ALARM_CALLBACK_OBJECT_ACE_TYPE   = 0x10

	SYSTEM_MANDATORY_LABEL_ACE_TYPE     = 0x11
	SYSTEM_RESOURCE_ATTRIBUTE_ACE_TYPE  = 0x12
	SYSTEM_SCOPED_POLICY_ID_ACE_TYPE    = 0x13
	SYSTEM_PROCESS_TRUST_LABEL_ACE_TYPE = 0x14
	SYSTEM_ACCESS_FILTER_ACE_TYPE       = 0x15
	ACCESS_MAX_MS_V5_ACE_TYPE           = 0x15
)

type AccessMask uint32

type ACE interface {
	GetSID() *windows.SID
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374919.aspx
type ACEHeader struct {
	ACEType  byte
	ACEFlags byte
	ACESize  uint16
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374847.aspx
type AccessAllowedACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *AccessAllowedACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374852.aspx
type AccessAllowedCallbackACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *AccessAllowedCallbackACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374854.aspx
type AccessAllowedCallbackObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *AccessAllowedCallbackObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374857.aspx
type AccessAllowedObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *AccessAllowedObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374879.aspx
type AccessDeniedACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *AccessDeniedACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374882.aspx
type AccessDeniedCallbackACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *AccessDeniedCallbackACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374882.aspx
type AccessDeniedCallbackObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *AccessDeniedCallbackObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa374887.aspx
type AccessDeniedObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *AccessDeniedObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379616.aspx
type SystemAuditACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *SystemAuditACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379617.aspx
type SystemAuditCallbackACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *SystemAuditCallbackACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379618.aspx
type SystemAuditCallbackObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *SystemAuditCallbackObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379619.aspx
type SystemAuditObjectACE struct {
	Header              ACEHeader
	Mask                AccessMask
	Flags               uint32
	ObjectType          windows.GUID
	InheritedObjectType windows.GUID
	SIDStart            uint32
}

func (a *SystemAuditObjectACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa965848.aspx
type SystemMandatoryLabelACE struct {
	Header   ACEHeader
	Mask     AccessMask
	SIDStart uint32
}

func (a *SystemMandatoryLabelACE) GetSID() *windows.SID {
	return (*windows.SID)(unsafe.Pointer(&a.SIDStart))
}
