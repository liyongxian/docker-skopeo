package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"

	gabs "github.com/Jeffail/gabs/v2"
)

func main() {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	configDir := filepath.Join(usr.HomeDir, ".docker")
	os.MkdirAll(configDir, os.ModePerm)

	configFile := filepath.Join(configDir, "config.json")
	// configFile := "test/config.json"

	jsonFile, err := os.OpenFile(configFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatal(err)
	}

	cfg := &gabs.Container{}
	if len(byteValue) == 0 {
		cfg, err = gabs.ParseJSON([]byte(`{}`))
	} else {
		cfg, err = gabs.ParseJSON(byteValue)
	}
	if err != nil {
		log.Fatal(err)
	}

	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "ECR_LOGIN_") {
			s := strings.Split(v, "=")
			cfg.Set("ecr-login", "credHelpers", s[1])
		}
	}

	// Docker Auth Configuration
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "DKR_AUTH_") {
			key := strings.Split(v, "=")[0]
			cnf := strings.Split(key, "__")
			if len(cnf) == 2 {
				if subkey := cnf[1]; len(subkey) != 0 {
					if repo := os.Getenv(cnf[0]); len(repo) != 0 {
						cfg.Set(getValue(key), "auths", repo, strings.ToLower(subkey))
					} else {
						log.Printf("WARN: Unable to find repo for %s", key)
						continue
					}
				}
			}
		}
	}

	cfgPretty := cfg.StringIndent("", "  ")

	jsonFile.Truncate(0)
	jsonFile.Seek(0, 0)
	jsonFile.Write([]byte(cfgPretty))

	if v := os.Getenv("DKRCFG_DEBUG"); len(v) != 0 {
		log.Printf("DEBUG: Docker Config: %s\n", configFile)
		log.Println(cfgPretty)
	}
}

func getValue(key string) string {
	val := os.Getenv(key)
	if v := os.Getenv("DKRCFG_ENABLE_AWS_PSTORE"); len(v) != 0 {
		if strings.HasPrefix(val, "arn:aws:ssm:") {
			return getParameter(val)
		}
	}
	return val
}

func getParameter(key string) (val string) {
	// Marshal Request
	prm := strings.Split(key, ":parameter")[1]
	region := strings.Split(key, ":")[3]

	// AWS Session
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config: *aws.NewConfig().WithRegion(region),
		// Profile: "tss_dev",
		// SharedConfigState: session.SharedConfigEnable,
	}))

	// SSM Client
	ssmclient := ssm.New(sess)
	resp, err := ssmclient.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(prm),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		log.Fatalf("ERROR: ssm.GetParameter:: %s\n%s", key, err)
	}
	val = *resp.Parameter.Value
	return
}
