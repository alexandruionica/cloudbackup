package password

import (
	"golang.org/x/crypto/bcrypt"
	"github.com/howeyc/gopass"
	"fmt"
	"unicode/utf8"
	"errors"
)


func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 5)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ReadPassFromCli() (string, error) {
	fmt.Println("Multi-byte characters (for example Unicode) not supported so please limit input to ASCII ! ")
	fmt.Printf("Password: ")
	pass, err := gopass.GetPasswdMasked()
	if err != nil {
		fmt.Printf("Encountered error while trying to read the password from the terminal. " +
			"The error was: %s \n", err)
		return "", err
	}
	if utf8.RuneCountInString(string(pass[:])) == 0 {
		msg := "Error! you have provided an empty password"
		fmt.Println(msg)
		return "", errors.New(msg)
	}
	hashedPass, err := HashPassword(string(pass[:]))
	if err != nil {
		fmt.Printf("Could not hash password. The encountered error was: %s \n", err)
		return "", err
	}
	return hashedPass, nil
}