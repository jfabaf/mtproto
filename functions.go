package mtproto

import (
	"fmt"
	"reflect"
	"log"
	"github.com/pkg/errors"
)


func (m *MTProto) Auth_SendCode(phonenumber string) (string, error) {
	var authSentCode TL_auth_sentCode
	flag := true
	for flag {
		resp := make(chan TL, 1)
		m.queueSend <- packetToSend{TL_auth_sendCode{
			Flags:          1,
			Current_number: TL_boolTrue{},
			Phone_number:   phonenumber,
			Api_id:         appId,
			Api_hash:       appHash,
		}, resp}
		x := <-resp
		switch x.(type) {
		case TL_auth_sentCode:
			authSentCode = x.(TL_auth_sentCode)
			flag = false
		case TL_rpc_error:
			x := x.(TL_rpc_error)
			if x.error_code != 303 {
				return "", fmt.Errorf("RPC error_code: %d", x.error_code)
			}
			var newDc int32
			n, _ := fmt.Sscanf(x.error_message, "PHONE_MIGRATE_%d", &newDc)
			if n != 1 {
				n, _ := fmt.Sscanf(x.error_message, "NETWORK_MIGRATE_%d", &newDc)
				if n != 1 {
					return "", fmt.Errorf("RPC error_string: %s", x.error_message)
				}
			}

			newDcAddr, ok := m.dclist[newDc]
			if !ok {
				return "", fmt.Errorf("Wrong DC index: %d", newDc)
			}
			err := m.reconnect(newDcAddr)
			fmt.Println("Reconnected")
			if err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("Got: %T", x)
		}

	}

	if authSentCode.Flags&1 == 0 {
		return "", errors.New("Cannot sign up yet")
	}

	return authSentCode.Phone_code_hash, nil
}

func (m *MTProto) Auth_SignIn(phonenumber string, hash, code string) error {
	resp := make(chan TL, 1)
	m.queueSend <- packetToSend{
		TL_auth_signIn{phonenumber, hash, code},
		resp,
	}
	x := <-resp
	auth, ok := x.(TL_auth_authorization)
	if !ok {
		return fmt.Errorf("RPC: %#v", x)
	}
	userSelf := auth.User.(TL_user)
	fmt.Printf("Signed in: id %d name <%s %s>\n", userSelf.Id, userSelf.First_name, userSelf.Last_name)
	return nil
}

func (m *MTProto) Auth_CheckPhone(phonenumber string) bool {
	resp := make(chan TL, 1)
	m.queueSend <- packetToSend{
		TL_auth_checkPhone{
			"989121228718",
		},
		resp,
	}
	x := <-resp
	if v, ok := x.(TL_auth_checkedPhone); ok {
		if toBool(v) {
			return true
		}
	}
	return false
}

func (m *MTProto) Contacts_GetContacts(hash int32) ([]Contact, []User) {
	resp := make(chan TL, 1)
	m.queueSend <- packetToSend{TL_contacts_getContacts{
		hash},
		resp,
	}
	x := <-resp
	list, ok := x.(TL_contacts_contacts)
	if !ok {
		log.Println(fmt.Sprintf("RPC: %#v", x))
		return []Contact{}, []User{}
	}
	TContacts := make([]Contact, 0, len(list.Contacts))
	TUsers := make([]User, 0, len(list.Users))
	for _, v := range list.Contacts {
		TContacts = append(
			TContacts,
			*NewContact(v),
		)
	}
	for _, v := range list.Users {
		switch u := v.(type) {
		case TL_user, TL_userEmpty:
			TUsers = append(TUsers, *NewUser(u))
		case TL_userProfilePhoto:
			TUsers[len(TUsers)-1].Photo = NewUserProfilePhoto(u)
		case TL_userStatusRecently, TL_userStatusOffline, TL_userStatusOnline, TL_userStatusLastWeek, TL_userStatusLastMonth:
			TUsers[len(TUsers)-1].Status = NewUserStatus(u)
		}
	}
	return TContacts, TUsers
}


func (m *MTProto) Messages_GetDialogs(offsetID, offsetDate, limit int32, offsetInputPeer TL) ([]Dialog, int) {
	resp := make(chan TL, 1)
	for {
		m.queueSend <- packetToSend{
			TL_messages_getDialogs{
				Offset_id:   offsetID,
				Offset_date: offsetDate,
				Limit:       limit,
				Offset_peer: offsetInputPeer,
			},
			resp,
		}
		x := <-resp
		mMessages := make(map[int32]*Message)
		mChats := make(map[int32]*Chat)
		mUsers := make(map[int32]*User)
		var dialogs []Dialog
		switch d := x.(type) {
		case TL_messages_dialogsSlice:
			for _, v := range d.Messages {
				m := NewMessage(v)
				mMessages[m.ID] = m
			}
			for _, v := range d.Chats {
				c := NewChat(v)
				mChats[c.ID] = c
			}
			for _, v := range d.Users {
				u := NewUser(v)
				mUsers[u.ID] = u
			}
			for _, v := range d.Dialogs {
				d := NewDialog(v)
				d.TopMessage = mMessages[d.TopMessageID]
				switch d.Type {
				case DIALOG_TYPE_USER:
					d.PeerAccessHash = mUsers[d.PeerID].AccessHash
					d.User = mUsers[d.PeerID]
				case DIALOG_TYPE_CHAT:
					d.Chat = mChats[d.PeerID]
				case DIALOG_TYPE_CHANNEL:
					d.PeerAccessHash = mChats[d.PeerID].AccessHash
					d.Chat = mChats[d.PeerID]
				}
				dialogs = append(dialogs, *d)
			}
			return dialogs, int(d.Count)
		case TL_messages_dialogs:
			for _, v := range d.Messages {
				m := NewMessage(v)
				mMessages[m.ID] = m
			}
			for _, v := range d.Chats {
				c := NewChat(v)
				mChats[c.ID] = c
			}
			for _, v := range d.Dialogs {
				d := NewDialog(v)
				d.TopMessage = mMessages[d.TopMessageID]
				switch d.Type {
				case DIALOG_TYPE_USER:
					d.PeerAccessHash = mUsers[d.PeerID].AccessHash
					d.User = mUsers[d.PeerID]
				case DIALOG_TYPE_CHAT:
					d.Chat = mChats[d.PeerID]
				case DIALOG_TYPE_CHANNEL:
					d.PeerAccessHash = mChats[d.PeerID].AccessHash
					d.Chat = mChats[d.PeerID]
				}
				dialogs = append(dialogs, *d)
			}
			return dialogs, len(d.Chats)
		default:
			return []Dialog{}, 0
		}
	}

}


func (m *MTProto) Upload_GetFile(in TL, offset, limit int32) []byte {
	resp := make(chan TL, 1)
	m.queueSend <- packetToSend{
		TL_upload_getFile{
			Offset:   offset,
			Limit:    limit,
			Location: in,
		},
		resp,
	}
	x := <-resp
	switch f := x.(type) {
	case TL_upload_file:
		return f.Bytes
	case TL_upload_fileCdnRedirect:

	case TL_rpc_error:
		if f.error_code == 303 {
			// Migrate Code
		}
	default:
		log.Println(reflect.TypeOf(f).String(), f)
	}
	return []byte{}
}

func (m *MTProto) Upload_GetCdnFile(fileToken []byte, offset, limit int32) []byte {
	resp := make(chan TL, 1)
	m.queueSend <- packetToSend{
		TL_upload_getCdnFile{
			fileToken,
			offset,
			limit,
		},
		resp,
	}
	x := <-resp
	switch f := x.(type) {
	case TL_upload_cdnFileReuploadNeeded:
		m.queueSend <- packetToSend{
			TL_upload_reuploadCdnFile{
				Request_token: f.Request_token,
				File_token:    fileToken,
			},
			resp,
		}
		z := <-resp
		switch reflect.TypeOf(z).Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(z)
			for i := 0; i < s.Len(); i++ {
				//hash := s.Interface().(TL_cdnFileHash)
				//TODO:: what to do now ?!!
			}
		}

	case TL_upload_cdnFile:
		return f.Bytes

	}

}
