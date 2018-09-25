package fileproperties

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
	"github.com/bearmini/go-acl/api"
	"golang.org/x/sys/windows"
)

var (
	ErrCouldNotGetSecurityInfo = errors.New("unable to get security information (file access control details)")
	ErrCouldNotGetOwner = errors.New("could not establish owner")
	ErrCouldNotGetGroup = errors.New("could not establish owning group")
	ErrCouldNotGetAccountDetails = errors.New("could not get account details")
	ErrCouldNotGetSidString = errors.New("could not obtain a string representation of the account SID")
	ErrUnsupportedAceType = errors.New("unsupported type of access control entry")
	ErrCouldNotJsonEncode = errors.New("could not json encode the object's permissions")
)

// Windows user/group account details
type Account struct {
	// see https://docs.microsoft.com/en-us/windows/desktop/secauthz/security-identifiers for what a SID is
	SID string
	// this is the name to which the above SID resolves - we'll try to use this when restoring if the SID doesn't exist
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
	ACEs []ACE
}

// gets in a platform dependent way the properties of a file or directory. The code here works only on NTFS file systems
// returns: owner name (string) ; FilePermissions object which was JSON Marshalled (string); error if != nil then the first
// two strings will be empty
//
// Example usage:
// 	_, jsonPermissions, err := getObjectPermissions(`C:\Users\bestygre\Desktop\test`)
//	if err != nil {
//		fmt.Printf("Got error: %s\n", err)
//	} else {
//		fmt.Printf("%+v\n", jsonPermissions)
//	}
//
func getObjectPermissions(path string) (string, string, error) {
	// Stuff to read to have a basic understanding of dACLS, ACEs and others:
	// https://docs.microsoft.com/en-us/windows/desktop/secauthz/dacls-and-aces
	var (
		owner   *windows.SID
		group   *windows.SID
		// discretionary access control list - (DACL) An access control list that is controlled by the owner of an
		// object and that specifies the access particular users or groups can have to the object.
		// dAcl header structure -  https://msdn.microsoft.com/ja-jp/library/windows/desktop/aa374931.aspx
		dAcl *api.ACL
		secDesc windows.Handle
		// this is the struct we're going to return
		filePerm FilePermissions
	)
	// handle paths longer than 260 chars (247 chars for folders)- see https://github.com/hectane/go-acl/issues/5
	// 1+2+256+1 or [drive][:\][path][null] = 260 (the last character is always a null terminator)
	// https://docs.microsoft.com/en-gb/windows/desktop/FileIO/naming-a-file#maxpath
	if utf8.RuneCountInString(path) > 247 && ! strings.HasPrefix(path, `\\?\`) {
		path = `\\?\` + path
	}

	fmt.Printf("######## examining '%s' ########\n", path)
	// field meaning - https://msdn.microsoft.com/en-us/library/windows/desktop/aa446645.aspx
	err := api.GetNamedSecurityInfo(
		path,
		api.SE_FILE_OBJECT,
		api.OWNER_SECURITY_INFORMATION | api.GROUP_SECURITY_INFORMATION | api.DACL_SECURITY_INFORMATION,
		&owner,
		&group,
		&dAcl,
		// TODO - figure out if we need the SACL too
		// system access control list - (SACL) An ACL that controls the generation of audit messages for attempts to
		// access a securable object. The ability to get or set an object's SACL is controlled by a privilege
		// typically held only by system administrators.
		nil,
		&secDesc,
	)
	if err != nil {
		// This `err` always contains "The operation completed successfully" ; this seems to be a Golang implementation
		// thing when using Win32API so we'll give a more generic error message
		fmt.Printf("unable to get security information (file access control details) for '%s'", path)
		return "", "", ErrCouldNotGetSecurityInfo
	}
	defer func(){
		_, err := windows.LocalFree(secDesc)
		if err != nil {
			fmt.Printf("Cound not free security descriptor for '%s'", path)
		}
	}()

	if owner == nil {
		fmt.Printf("could not establish owner for '%s'", path)
		return "", "", ErrCouldNotGetOwner
	}

	filePerm.Owner.SID, err = owner.String()
	if err != nil {
		fmt.Printf("while trying to get the string representation of the account SID representing the owner of " +
			"'%s' the following error was encountered: '%s'", path, err)
		return "", "", ErrCouldNotGetSidString
	}

	filePerm.Owner.Name, filePerm.Owner.Domain, filePerm.Owner.Type, err = owner.LookupAccount("")
	if err != nil {
		fmt.Printf("while trying to get account details for '%s' which is the owner of '%s' the " +
			"following error was encountered: '%s'", filePerm.Owner.SID, path, err)
		return "", "", ErrCouldNotGetAccountDetails
	}

	if group == nil {
		fmt.Printf("could not establish owning group for '%s'", path)
		return "", "", ErrCouldNotGetGroup
	}
	filePerm.Group.SID, err = group.String()
	if err != nil {
		fmt.Printf("while trying to get the string representation of the account SID representing the group of " +
			"'%s' the following error was encountered: '%s'", path, err)
		return "", "", ErrCouldNotGetSidString
	}

	filePerm.Group.Name, filePerm.Group.Domain, filePerm.Group.Type, err = group.LookupAccount("")
	if err != nil {
		fmt.Printf("while trying to get account details for '%s' which is the owning group of '%s' the " +
			"following error was encountered: '%s'", filePerm.Group.SID, path, err)
		return "", "", ErrCouldNotGetAccountDetails
	}

	if dAcl == nil {
		// debugmsg
		fmt.Printf("'%s' doesn't have a DACL\n", path)
		jsonPayload, err := json.Marshal(filePerm)
		if err !=nil {
			return "", "", ErrCouldNotJsonEncode
		}
		return filePerm.Owner.Name, string(jsonPayload), nil
	}

	// numAces := dAcl.ACECount
	aces := dAcl.GetACEList()
	// start from 1 not 0 as output is shown in error messages
	currentAceNumber := 1
	for _, ace := range aces {
		sidAccountSid, err := ace.GetSID().String()
		if err != nil {
			fmt.Printf("while trying to get the string representation of the account SID for ACL " +
				"entry %d belonging to '%s' the following error was encountered: '%s'", currentAceNumber , path, err)
			return "", "", ErrCouldNotGetSidString
		}
		sidAccountName, sidDomain, sidAccountType, err := ace.GetSID().LookupAccount("")
		if err != nil {
			fmt.Printf("unable to get security details for ACL entry %d having details '%+v' of '%s' as the " +
				"following error was encountered: '%s'", currentAceNumber, ace, path, err)
			return "", "", ErrCouldNotGetAccountDetails
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
				fmt.Printf("Unsupported Access Control Entry of type: '%+v'", aceDetails)
				return "", "", ErrUnsupportedAceType
			}
		}
		filePerm.ACEs = append(filePerm.ACEs, entry)
	}
	fmt.Printf("%+v\n", filePerm)
	fmt.Printf("#####\n")

	// when restoring ACEs note that the order of the ACEs is important because the system reads the ACEs in
	// sequence until access is granted or denied. The user's access-denied ACE must appear first; otherwise, when the
	// system reads the group's access allowed ACE, it will grant access to the restricted user
	jsonPayload, err := json.Marshal(filePerm)
	if err !=nil {
		fmt.Printf("Could not JSON encode the permissions of '%s' due to error: '%s'", path, err)
		return "", "", ErrCouldNotJsonEncode
	}
	return filePerm.Owner.Name, string(jsonPayload), nil
}

