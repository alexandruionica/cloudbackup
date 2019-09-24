package fileproperties

import (
	"encoding/json"
	"github.com/bearmini/go-acl/api"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"unicode/utf8"
)

// Windows user/group account details
type Account struct {
	// see https://docs.microsoft.com/en-us/windows/desktop/secauthz/security-identifiers for what a SID is
	SID string
	// this is the name to which the above SID resolves - we'll try to use this when restoring if the SID doesn't exist.
	// If the SID can't be resolved to a name then the SIDs value will be set in the name field too
	Name string
	// name of the first domain on which this SID is found
	Domain string
	/* see peUse from https://docs.microsoft.com/en-us/windows/desktop/api/winbase/nf-winbase-lookupaccountsidw and
	what each value means is at https://docs.microsoft.com/en-gb/windows/desktop/api/winnt/ne-winnt-_sid_name_use
	At https://www.rubydoc.info/gems/win32-security/0.2.3/Windows/Security/Constants you can find a constant to
	value mapping for each account type */
	Type uint32
}

// TODO - support more fields beside the AceHeader (AceType + AceFlags) and AceMask . For example the library we use,
//  github.com/bearmini/go-acl/api has implemented support for them and looking at
// https://docs.microsoft.com/en-gb/windows/desktop/api/winnt/ns-winnt-_ace_header shows that there are way more fields
// available for some of the AceType
type ACE struct {
	// see AceType in https://docs.microsoft.com/en-gb/windows/desktop/api/winnt/ns-winnt-_ace_header for meaning
	// and in the library https://github.com/bearmini/go-acl/blob/master/api/ace.go#L12 the constant to value mapping
	Type byte
	// see AceFlags in https://docs.microsoft.com/en-gb/windows/desktop/api/winnt/ns-winnt-_ace_header for meaning
	Flags byte
	// see https://docs.microsoft.com/en-gb/windows/desktop/SecAuthZ/access-mask
	Mask uint32
	Account
}

type FilePermissions struct {
	Owner Account
	Group Account
	ACEs  []ACE
}

// gets in a platform dependent way the properties of a file or directory. The code here works only on NTFS file systems
// parameters: $path is the path to the file/directory ; $stat - not used in the Windows implementation
// returns: owner name (string) ; FilePermissions object which was JSON Marshalled (string); error if != nil then the first
// strings will container if possible the account name and if this can't be extracted then the account SID or worst
// case scenario it will be empty; the second string will be empty if an error is encountered
//
// Example usage:
// 	_, jsonPermissions, err := GetObjectPermissions(`C:\Users\testuser\Desktop\test`)
//	if err != nil {
//		fmt.Printf("Got error: %s\n", err)
//	} else {
//		fmt.Printf("%+v\n", jsonPermissions)
//	}
//
func GetObjectPermissions(path string, stat os.FileInfo) (string, string, error) {
	// Stuff to read to have a basic understanding of dACLS, ACEs and others:
	// https://docs.microsoft.com/en-us/windows/desktop/secauthz/dacls-and-aces
	var (
		owner *windows.SID
		group *windows.SID
		// discretionary access control list - (DACL) An access control list that is controlled by the owner of an
		// object and that specifies the access particular users or groups can have to the object.
		// dAcl header structure -  https://msdn.microsoft.com/ja-jp/library/windows/desktop/aa374931.aspx
		dAcl    *api.ACL
		secDesc windows.Handle
		// this is the struct we're going to return
		filePerm FilePermissions
	)
	// handle paths longer than 260 chars (247 chars for folders)- see https://github.com/hectane/go-acl/issues/5
	// 1+2+256+1 or [drive][:\][path][null] = 260 (the last character is always a null terminator)
	// https://docs.microsoft.com/en-gb/windows/desktop/FileIO/naming-a-file#maxpath
	if utf8.RuneCountInString(path) > 247 && !strings.HasPrefix(path, `\\?\`) {
		path = `\\?\` + path
	}

	// field meaning - https://msdn.microsoft.com/en-us/library/windows/desktop/aa446645.aspx
	err := api.GetNamedSecurityInfo(
		path,
		api.SE_FILE_OBJECT,
		api.OWNER_SECURITY_INFORMATION|api.GROUP_SECURITY_INFORMATION|api.DACL_SECURITY_INFORMATION,
		&owner,
		&group,
		&dAcl,
		// decided not to fetch the SACL. The SACL should be managed via GPOs and the GPO should be reapplied after a restore
		// system access control list - (SACL) An ACL that controls the generation of audit messages for attempts to
		// access a securable object. The ability to get or set an object's SACL is controlled by a privilege
		// typically held only by system administrators.
		nil,
		&secDesc,
	)
	if err != nil {
		// This `err` always contains "The operation completed successfully" ; this seems to be a Golang implementation
		// thing when using Win32API so we'll give a more generic error message
		logger.Warnf("unable to get security information (file access control details) for '%s'", path)
		return "", "", ErrCouldNotGetSecurityInfo
	}
	defer func() {
		_, err := windows.LocalFree(secDesc)
		if err != nil {
			logger.Warnf("Could not free security descriptor for '%s'", path)
		}
	}()

	if owner == nil {
		logger.Warnf("could not establish owner for '%s'", path)
		return "", "", ErrCouldNotGetOwner
	}

	filePerm.Owner.SID = owner.String()

	filePerm.Owner.Name, filePerm.Owner.Domain, filePerm.Owner.Type, err = owner.LookupAccount("")
	if err != nil {
		logger.Warnf("while trying to get account details for '%s' which is the owner of '%s' the "+
			"following error was encountered: '%s'", filePerm.Owner.SID, path, err)
		return filePerm.Owner.SID, "", ErrCouldNotGetAccountDetails
	}
	if filePerm.Owner.Name == "" {
		filePerm.Owner.Name = filePerm.Owner.SID
	}

	if group == nil {
		logger.Warnf("could not establish owning group for '%s'", path)
		return filePerm.Owner.Name, "", ErrCouldNotGetGroup
	}
	filePerm.Group.SID = group.String()

	filePerm.Group.Name, filePerm.Group.Domain, filePerm.Group.Type, err = group.LookupAccount("")
	if err != nil {
		logger.Warnf("while trying to get account details for '%s' which is the owning group of '%s' the "+
			"following error was encountered: '%s'", filePerm.Group.SID, path, err)
		return filePerm.Owner.Name, "", ErrCouldNotGetAccountDetails
	}
	if filePerm.Group.Name == "" {
		filePerm.Group.Name = filePerm.Group.SID
	}

	if dAcl == nil {
		// debugmsg
		logger.Infof("'%s' doesn't have a DACL\n", path)
		jsonPayload, err := json.Marshal(filePerm)
		if err != nil {
			return filePerm.Owner.Name, "", ErrCouldNotJsonEncode
		}
		return filePerm.Owner.Name, string(jsonPayload), nil
	}

	// numAces := dAcl.ACECount
	aces := dAcl.GetACEList()
	// start from 1 not 0 as output is shown in error messages
	currentAceNumber := 1
	for _, ace := range aces {
		sidAccountSid := ace.GetSID().String()
		sidAccountName, sidDomain, sidAccountType, err := ace.GetSID().LookupAccount("")
		if err != nil {
			logger.Warnf("unable to get security details for ACL entry %d having details '%+v' of '%s' as the "+
				"following error was encountered: '%s'", currentAceNumber, ace, path, err)
			return filePerm.Owner.Name, "", ErrCouldNotGetAccountDetails
		}

		entry := ACE{
			Account: Account{
				SID:    sidAccountSid,
				Name:   sidAccountName,
				Domain: sidDomain,
				Type:   sidAccountType,
			},
		}

		switch aceDetails := ace.(type) {
		case *api.AccessAllowedACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessAllowedCallbackACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessAllowedCallbackObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessAllowedObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessDeniedACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessDeniedCallbackACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessDeniedCallbackObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.AccessDeniedObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.SystemAuditACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.SystemAuditCallbackACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.SystemAuditCallbackObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.SystemAuditObjectACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		case *api.SystemMandatoryLabelACE:
			{
				entry.Type = aceDetails.Header.ACEType
				entry.Mask = uint32(aceDetails.Mask)
				entry.Flags = aceDetails.Header.ACEFlags
			}
		default:
			{
				logger.Warnf("Unsupported Access Control Entry of type: '%+v'", aceDetails)
				return filePerm.Owner.Name, "", ErrUnsupportedAceType
			}
		}
		filePerm.ACEs = append(filePerm.ACEs, entry)
	}
	logger.Debugf("permissions of '%s' are: %+v\n", path, filePerm)

	// when restoring ACEs note that the order of the ACEs is important because the system reads the ACEs in
	// sequence until access is granted or denied. The user's access-denied ACE must appear first; otherwise, when the
	// system reads the group's access allowed ACE, it will grant access to the restricted user
	jsonPayload, err := json.Marshal(filePerm)
	if err != nil {
		logger.Warnf("Could not JSON encode the permissions of '%s' due to error: '%s'", path, err)
		return filePerm.Owner.Name, "", ErrCouldNotJsonEncode
	}
	return filePerm.Owner.Name, string(jsonPayload), nil
}
