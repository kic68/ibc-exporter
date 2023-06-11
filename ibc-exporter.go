package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/goccy/go-yaml"

	"go.bug.st/serial"

	"github.com/howeyc/crc16"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/juju/loggo"
	"github.com/juju/loggo/loggocolor"

	"github.com/coreos/go-systemd/daemon"
)

var (
	config = map[string]string{}

	logger = loggo.GetLogger("")

	listenPort = "5042"
	c          = map[string]string{}
	m          = [][]string{
		// p30
		{"totalEnergyProduction", "0102", "Global", "Total Energy Production in Wh"},
		{"energyProductionToday", "0104", "Global", "Energy Production Today in Wh"},

		// p31
		{"pvVoltageInput", "0228", "Input1", "PV Voltage in V/10"},
		{"pvVoltageInput", "0229", "Input2", "PV Voltage in V/10"},
		{"pvVoltageInput", "022a", "Input3", "PV Voltage in V/10"},

		{"pvCurrent", "022d", "Input1", "PV Current in mA"},
		{"pvCurrent", "022e", "Input2", "PV Current in mA"},
		{"pvCurrent", "022f", "Input3", "PV Current in mA"},

		{"pvPower", "0232", "Input1", "PV Power in W"},
		{"pvPower", "0233", "Input2", "PV Power in W"},
		{"pvPower", "0234", "Input3", "PV Power in W"},

		{"pvEnergy", "0237", "Input1", "PV Energy in Wh"},
		{"pvEnergy", "0238", "Input2", "PV Energy in Wh"},
		{"pvEnergy", "0239", "Input3", "PV Energy in Wh"},

		{"gridVoltage", "023c", "L1", "Grid Voltage in V/10"},
		{"gridVoltage", "023d", "L2", "Grid Voltage in V/10"},
		{"gridVoltage", "023e", "L3", "Grid Voltage in V/10"},

		// p32
		{"gridVoltage10MinMean", "025b", "L1", "Grid Voltage 10 Minutes Mean in V/10"},
		{"gridVoltage10MinMean", "025c", "L2", "Grid Voltage 10 Minutes Mean in V/10"},
		{"gridVoltage10MinMean", "025d", "L3", "Grid Voltage 10 Minutes Mean in V/10"},

		{"gridVoltage", "025e", "L1L2", "Grid Voltage in V/10"},
		{"gridVoltage", "025f", "L2L3", "Grid Voltage in V/10"},
		{"gridVoltage", "0260", "L3L1", "Grid Voltage in V/10"},

		{"gridCurrent", "023f", "L1", "Grid Current in mA"},
		{"gridCurrent", "0240", "L2", "Grid Current in mA"},
		{"gridCurrent", "0241", "L3", "Grid Current in mA"},

		{"gridPower", "0242", "L1", "Grid Power in W"},
		{"gridPower", "0243", "L2", "Grid Power in W"},
		{"gridPower", "0244", "L3", "Grid Power in W"},

		{"gridPowerSum", "0246", "L1L2L3", "Grid Power Sum in W"},

		{"gridEnergyToday", "0247", "L1", "Grid Energy Today in Wh"},
		{"gridEnergyToday", "0248", "L2", "Grid Energy Today in Wh"},
		{"gridEnergyToday", "0249", "L3", "Grid Energy Today in Wh"},
		{"gridEnergyTodaySum", "024a", "L1L2L3", "Grid Energy Today in Wh"},
		{"gridEnergyTodayExternalMeter", "024b", "s0", "Grid Energy, external Meter in Wh"},

		{"gridCurrentDCContent", "024c", "L1", "Grid current, DC content, in mA"},
		{"gridCurrentDCContent", "024d", "L2", "Grid current, DC content, in mA"},
		{"gridCurrentDCContent", "024e", "L3", "Grid current, DC content, in mA"},

		{"gridFrequency", "0261", "L1", "Grid Freqency in MHz"},
		{"gridFrequency", "0262", "L2", "Grid Freqency in MHz"},
		{"gridFrequency", "0263", "L3", "Grid Freqency in MHz"},

		// p33
		{"gridFrequencyMean", "0250", "L1L2L3", "Grid Frequency Mean in mHz"},

		{"globalIrradiance", "0202", "Global", "Global Irradiance in W/m2"},
		{"totalGlobalIrradiation", "0206", "Global", "Total Global Irradiation in Wh/m2"},
		{"irradianceSensorTemperature", "0205", "", "Irradiance Sensor Temperature in °C"},
		{"ambientTemperature", "0203", "Global", "Ambient Temperature in °C"},
		{"pvArrayTemperature", "0204", "Global", "Array Temperature in °C"},

		// p34
		{"operationMode", "0a02", "Global", "Operation Mode"}, //  see below
		/*
			00-09 The complete inverter is shut down
			10-49 The inverter is booting, initializing itself etc.
			50-59 The inverter is monitoring the grid, preparing to connect.
			60-69 The inverter is connected to grid and is producing energy.
			70-79 The inverter is disconnected from grid because of an error situation.
			80-89 The inverter is shut down (except for the user interface and the communication interfaces).
		*/
		{"latestEvent", "0a28", "Global", "Latest Event Code, cleared on Grid Connect"}, //  see below
		/*
			Event category         Event code intervals
			Grid                   1-37, 40-99, 246
			PV                     103-111, 115-200
			Fail safe              38-39, 100-102, 112-114, 225-244, 248-251, 350-400
			Internal               201-211, 213-221, 222-224, 247, 252-299, 212
			De-rate                300-349
		*/
		// p37 + p38
		{"powerClass", "1e14", "Global", "Power Class of Inverter"},
		{"nominalACPower", "4701", "Global", "Nominal AC Power in W"},
		{"softwarePackageVersion", "3228", "Global", "Software Package Version in /100"},

		// p39 ff
		{"inverterProduction", "780a", "Today", "Inverter Production in Wh"},
		{"inverterProduction", "780b", "LastMonday", "Inverter Production in Wh"},
		{"inverterProduction", "780c", "LastTuesday", "Inverter Production in Wh"},
		{"inverterProduction", "780d", "LastWednesday", "Inverter Production in Wh"},
		{"inverterProduction", "780e", "LastThursday", "Inverter Production in Wh"},
		{"inverterProduction", "780f", "LastFriday", "Inverter Production in Wh"},
		{"inverterProduction", "7810", "LastSaturday", "Inverter Production in Wh"},
		{"inverterProduction", "7811", "LastSunday", "Inverter Production in Wh"},

		{"inverterProduction", "7814", "ThisWeek", "Inverter Production in Wh"},
		{"inverterProduction", "7815", "LastWeek", "Inverter Production in Wh"},
		{"inverterProduction", "7816", "2WeeksAgo", "Inverter Production in Wh"},
		{"inverterProduction", "7817", "3WeeksAgo", "Inverter Production in Wh"},
		{"inverterProduction", "7818", "4WeeksAgo", "Inverter Production in Wh"},
	}

	port       serial.Port
	inverters  []string
	exposed    = map[string]*prometheus.GaugeVec{}
	lastUpdate = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ibc_lastUpdate",
		Help: "Last update timestamp in epoch seconds",
	},
		[]string{"inverter", "scope"},
	)
)

func init() {

	loggo.ConfigureLoggers("<root>=ERROR")
	loggo.ReplaceDefaultWriter(loggocolor.NewColorWriter(os.Stderr))

	c["start"] = "7e" // always 7e
	c["addr"] = "ff"  // broadcast
	c["ctrl"] = "03"
	c["stop"] = "7e" // always 7e
	c["me"] = "0002"
	c["broadcast"] = "ffff"
	c["stop"] = "7e"

	for _, array := range m {
		// fmt.Printf("Querying %s: ", metric)
		metric := array[0]
		help := array[3]
		//fmt.Printf("%s\n", metric)

		_, ok := exposed[metric]
		if !ok {
			exposed[metric] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ibc_" + metric,
				Help: help,
			},
				[]string{"inverter", "scope"},
			)

			prometheus.MustRegister(exposed[metric])
		}
	}

	prometheus.MustRegister(lastUpdate)

}

func createMessage(message string) string {
	logger.Tracef(trace())

	// https://de.wikipedia.org/wiki/High-Level_Data_Link_Control
	// https://de.wikipedia.org/wiki/Zyklische_Redundanzprüfung CRC-CCITT (CRC16)
	// https://www.scadacore.com/tools/programming-calculators/online-hex-converter/

	logger.Debugf(fmt.Sprintf("Pure Message: %v\n", message))
	message = c["addr"] + c["ctrl"] + message

	fullMessage := message + calculateFCS(message)

	// byte stuffing (replace 7d/7e)

	// 7d
	fullMessage7d := ""
	for i := 0; i < len(fullMessage); i += 2 {
		hexdigits := fullMessage[i : i+2]
		if hexdigits == "7d" {
			fullMessage7d += "7d5d"
		} else {
			fullMessage7d += hexdigits
		}
	}

	fullMessage = ""
	for i := 0; i < len(fullMessage7d); i += 2 {
		hexdigits := fullMessage7d[i : i+2]
		if hexdigits == "7e" {
			fullMessage += "7d5e"
		} else {
			fullMessage += hexdigits
		}
	}

	fullMessage = c["start"] + fullMessage + c["stop"]
	logger.Debugf(fmt.Sprintf("Full Message: %v\n", fullMessage))
	return fullMessage
}

func calculateFCS(message string) string {
	logger.Tracef(trace())

	logger.Debugf(fmt.Sprintf("Calculating FCS from: %v\n", message))

	binMessage, err := hex.DecodeString(message)
	if err != nil {
		logger.Errorf(err.Error())
	}
	// Calculate FCS
	fcsUint := crc16.ChecksumCCITT(binMessage)
	logger.Debugf(fmt.Sprintf("Int FCS: %v\n", fcsUint))
	// Convert to Hex
	fcsHexSwapped := strconv.FormatUint(uint64(fcsUint), 16)
	logger.Debugf(fmt.Sprintf("Swapped FCS: %v\n", fcsHexSwapped))
	logger.Debugf(fmt.Sprintf("Length Swapped FCS: %v\n", len(fcsHexSwapped)))
	// Pad with leading zeroes
	fcsHexSwapped = "0000"[0:4-len(fcsHexSwapped)] + fcsHexSwapped
	logger.Debugf(fmt.Sprintf("Padded Swapped FCS: %v\n", fcsHexSwapped))
	// Swap bytes
	fcsHex := fcsHexSwapped[2:4] + fcsHexSwapped[0:2]

	logger.Debugf(fmt.Sprintf("Calculating FCS: %v\n", fcsHex))

	return fcsHex
}

func getPort() serial.Port {
	logger.Tracef(trace())

	ports, err := serial.GetPortsList()
	if err != nil {
		logger.Errorf(err.Error())
	}
	if len(ports) == 0 {
		logger.Errorf("No serial ports found!")
	}

	usePort := ""
	for _, port := range ports {
		logger.Debugf(fmt.Sprintf("Found port: %v\n", port))
		match, _ := regexp.MatchString(".*ttyUSB.*", port)
		if match {
			usePort = port
			logger.Debugf(fmt.Sprintf("Using port: %v\n", usePort))
			break
		}
	}
	if len(usePort) == 0 {
		logger.Errorf("No USB serial ports found!")
	}

	mode := &serial.Mode{
		BaudRate: 19200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(usePort, mode)
	if err != nil {
		logger.Errorf(err.Error())
	}

	return port
}

func communicateWithPort(message string) (string, string, string) {
	logger.Tracef(trace())

	fullMessage := createMessage(message)
	sendToPort(fullMessage)
	fullAnswer := readFromPort()
	logger.Debugf(fmt.Sprintf("Length of answer: %d\n", len(fullAnswer)))
	// byte unstuffing
	// Remove leading/trailing 7eff03 / 7e
	// NodeInfo
	// 7eff032c3700021d93202020313339463031383000203035333830324e33313200020c37000041f87e

	// remove leading/trailing 7e
	fullAnswer7e := ""
	for i := 0; i < len(fullAnswer); i += 2 {
		hexdigits := fullAnswer[i : i+2]
		if hexdigits == "7e" {
			fullAnswer7e += ""
		} else {
			fullAnswer7e += hexdigits
		}
	}

	// Replace 7d5e by 7e
	fullAnswer7d5e := ""
	for i := 0; i < len(fullAnswer7e)-2; i += 2 {
		hexdigits := fullAnswer7e[i : i+4]
		if hexdigits == "7d5e" {
			fullAnswer7d5e += "7e"
			i += 2
		} else {
			if i < len(fullAnswer7e)-4 {
				fullAnswer7d5e += hexdigits[0:2]
			} else {
				fullAnswer7d5e += hexdigits[0:4]
			}
		}
	}

	// Replace 7d5d by 7d
	fullAnswer = ""
	for i := 0; i < len(fullAnswer7d5e)-2; i += 2 {
		hexdigits := fullAnswer7d5e[i : i+4]
		if hexdigits == "7d5d" {
			fullAnswer += "7d"
			i += 2
		} else {
			if i < len(fullAnswer7e)-4 {
				fullAnswer += hexdigits[0:2]
			} else {
				fullAnswer += hexdigits[0:4]
			}
		}
	}

	// ff032c3700021d93202020313339463031383000203035333830324e33313200020c37000041f8
	refcs := regexp.MustCompile(`^(.*)(....)$`)
	fcs := refcs.ReplaceAllString(fullAnswer, "$2")
	fullAnswer = refcs.ReplaceAllString(fullAnswer, "$1")
	logger.Debugf(fmt.Sprintf("Provided AnswerFCS: %v\n", fcs))
	calculatedFCS := calculateFCS(fullAnswer)
	logger.Debugf(fmt.Sprintf("Calculated AnswerFCS: %v\n", calculatedFCS))
	reff03del := regexp.MustCompile(`ff03`)
	fullAnswer = reff03del.ReplaceAllString(fullAnswer, "")

	// TODO Continue to split
	resplit := regexp.MustCompile(`^(....)(....)(.*)$`)
	splitted := resplit.FindStringSubmatch(fullAnswer)

	logger.Debugf(fmt.Sprintln("Elements:", len(splitted), "Replaced FullAnswer:", len(fullAnswer)))
	// entire message, un-regexed
	_ = splitted[0]
	src := splitted[1]
	dst := splitted[2]
	canbus := splitted[3]
	return src, dst, canbus
}

func sendToPort(fullMessage string) {
	logger.Tracef(trace())

	binMessage, err := hex.DecodeString(fullMessage)
	if err != nil {
		logger.Errorf(err.Error())
	}

	n, err := port.Write([]byte(binMessage))
	if err != nil {
		logger.Errorf(err.Error())
	}
	logger.Debugf("Sent %v bytes to port\n", n)
}

func readFromPort() string {
	logger.Tracef(trace())

	time.Sleep(time.Second / 5)
	// Reads up to 100 bytes
	buff := make([]byte, 100)
	readBytes, err := port.Read(buff)

	if err != nil {
		logger.Errorf(err.Error())
	}
	hexAnswer := hex.EncodeToString(buff[:readBytes])
	logger.Debugf(fmt.Sprintf("Read %v bytes from port\n", readBytes))
	logger.Debugf(fmt.Sprintf("Answer from port: %v\n", hexAnswer))
	return hexAnswer
}

func ping(dst string) string {
	logger.Tracef(trace())

	// c["broadcast_ping"] = "0002ffff0015"

	// src, dst, pingAnswer
	src, _, _ := communicateWithPort(c["me"] + dst + "0015")
	return src
}

func nodeInformation(dst string) (string, string) {
	logger.Tracef(trace())

	// c["node_information"] = "00022c371d13"

	// src, dst, nodeInformation
	_, _, nodeInformation := communicateWithPort(c["me"] + dst + "1d13ffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	resplit := regexp.MustCompile(`^(..)(..)(.{22})(..)(.{22})(..).*$`)
	splitted := resplit.FindStringSubmatch(nodeInformation)
	// length := splitted[1]             // 0x1D = 29
	// messageType := splitted[2]        // 0x13
	productNumber := splitted[3] // 11 characters represented in ASCII codes, nused characters must be set to the value 0x20
	// stringTermination1 := splitted[4] // 0x00
	serialNumber := splitted[5] // 11 characters represented in ASCII codes, nused characters must be set to the value 0x20
	// stringTermination2 := splitted[6] // 0x00
	decodedProductNumber, _ := hex.DecodeString(productNumber)
	decodedSerialNumber, _ := hex.DecodeString(serialNumber)
	logger.Debugf(fmt.Sprintf("Product Number: %v / Serial Number: %v\n", string(decodedProductNumber), string(decodedSerialNumber)))
	return string(decodedProductNumber), string(decodedSerialNumber)
}

func canRequest(dst string, parameterIndexSubIndex string) string {
	logger.Tracef(trace())

	// embedded
	message := "0a" // length 10
	message += "01" // embedded canbus message
	message += "c8" // canbus document number (always 200)
	message += "0"  // destination module id, first part always 0
	message += "8"  // destination module id, second part. Always 8 in TripleLynx
	message += "d"  // source module id, 0xd for rs485
	message += "0"  // page number is always 0
	message += parameterIndexSubIndex
	message += "80"       // flags 1000 datatype 0000
	message += "00000000" // flags 1000 datatype 0000

	_, _, canResponse := communicateWithPort(c["me"] + dst + message)

	// Sometimes the ending 00 are missing
	canResponse += "00000000"
	resplit := regexp.MustCompile(`^(..)(..)(..)(.)(.)(.)(.)(..)(..)(.)(.)(..)(..)(..)(..).*$`)
	splitted := resplit.FindStringSubmatch(canResponse)

	var parameterValue string
	if len(splitted) != 16 {
		logger.Warningf("----> Split did not return enough slices:", len(splitted), canResponse)
		parameterValue = "00000000"
	} else {
		// length := splitted[1]             // 0x0a = 10
		// messageType := splitted[2]        // 0x81
		// documentNumber := splitted[3] 	 // c8
		// unpredictable := splitted[4]
		// requestorID := splitted[5]
		//inverterModule := splitted[6]
		// unused := splitted[7]
		// parameterIndex := splitted[8]
		// parameterSubIndex := splitted[9]
		// flags := splitted[10]
		// dataTypeID := splitted[11]
		parameterValue = splitted[15] + splitted[14] + splitted[13] + splitted[12]
		// fmt.Println(dataTypeID, parameterValue)

		byteArrayParameterValue, _ := hex.DecodeString(parameterValue)

		parameterValue = strconv.FormatUint(uint64(binary.BigEndian.Uint32(byteArrayParameterValue)), 10)

	}
	return parameterValue
}

func trace() string {
	pc, file, line, ok := runtime.Caller(1)
	if !ok {
		return "TRACE ERROR"
	}

	fn := runtime.FuncForPC(pc)
	return fmt.Sprintf("File: %s Line: %d Function: %s", file, line, fn.Name())
}

func getCredentials(credentialsFile string) {

	osFile, err := os.Open(credentialsFile)
	if err != nil {
		logger.Infof(fmt.Sprintf("Couldn't read credentials file: %s", err.Error()))
	}

	err = yaml.NewDecoder(osFile).Decode(&config)

	config["mqtt"] = "ok"

	if err != nil {
		logger.Errorf(fmt.Sprintf("Couldn't parse config file: %s", err.Error()))
		config["mqtt"] = "impossible"
	}

	_, ok := config["userName"]
	if !ok {
		logger.Errorf("userName missing.")
		config["mqtt"] = "impossible"
	}
	_, ok = config["password"]
	if !ok {
		logger.Errorf("password missing.")
		config["mqtt"] = "impossible"
	}
	_, ok = config["mqttAddress"]
	if !ok {
		logger.Errorf("mqttAddress missing.")
		config["mqtt"] = "impossible"
	}
	_, ok = config["clientName"]
	if !ok {
		logger.Errorf("clientName missing.")
		config["mqtt"] = "impossible"
	}
	if config["mqtt"] != "ok" {
		logger.Errorf("YAML file needs to have this structure:\n\n---\nuserName: valUserName\npassword: valPassword\nmqttAddress: \"tcp://host:1883\"\nclientName: valClientName\n\nNo MQTT publishing will be active")
	} else {
		logger.Errorf("MQTT publishing active!")
	}
}

func publishMqtt(trimmedSerialNumber string, scope string, metric string, value int) {
	if config["mqtt"] == "ok" {

		mqtt.ERROR = log.New(os.Stdout, "", 0)
		opts := mqtt.NewClientOptions().AddBroker(config["mqttAddress"]).SetClientID(config["clientName"])
		opts.SetUsername(config["userName"])
		opts.SetPassword(config["password"])
		opts.SetKeepAlive(2 * time.Second)
		opts.SetPingTimeout(1 * time.Second)

		client := mqtt.NewClient(opts)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			logger.Debugf(fmt.Sprintf("Connection to broker failed: %s", token.Error()))
		} else {
			logger.Infof(fmt.Sprintf("publishMqtt: pushing to ibc/%s/%s/%s value: %d\n", trimmedSerialNumber, scope, metric, value))
			token := client.Publish("ibc/"+trimmedSerialNumber+"/"+scope+"/"+metric, 0, true, strconv.Itoa(value))
			token.Wait()

			client.Disconnect(250)
		}
	}
}

func main() {

	if len(os.Args) > 1 {
		getCredentials(os.Args[1])
	} else {
		logger.Errorf(fmt.Sprintf("If you want MQTT logging, add path to configuration file as first argument to program: %s /path/to/config_file", os.Args[0]))
		getCredentials("undefined_path_and_file")
	}
	fmt.Println("\nLogging level:")
	fmt.Println(loggo.LoggerInfo())
	fmt.Println("")

	// Create channel to communicate with subroutine
	c1 := make(chan string, 1)

	// Subroutine for Metrics HTTP server
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":"+listenPort, nil)

	// readyness check
	daemon.SdNotify(false, daemon.SdNotifyReady)

	port = getPort()
	// Broadcast Ping to RS485 -> find all inverters. Loop to be programmed later :)
	inverters = append(inverters, ping(c["broadcast"]))
	logger.Infof(fmt.Sprintf("Broadcast Ping Answer: %s\n", inverters[0]))
	for {
		for _, inverter := range inverters {
			logger.Infof(fmt.Sprintf("Ping Answer: %s\n", ping(inverter)))
			productNumber, serialNumber := nodeInformation(inverter)
			trimmedProductNumber := strings.TrimSpace(productNumber)
			trimmedSerialNumber := strings.TrimSpace(serialNumber)

			logger.Infof(fmt.Sprintf("Product Number: %s / Serial Number: %s\n", trimmedProductNumber, trimmedSerialNumber))

			now := time.Now()
			seconds := now.Unix()
			lastUpdate.WithLabelValues(trimmedSerialNumber, "Global").Set(float64(seconds))

			//mqttValues := map[string]
			for _, array := range m {
				metric := array[0]
				parameter := array[1]
				scope := array[2]
				// fmt.Printf("Querying %s: ", metric)

				go func() {
					logger.Infof("========= Starting parameter query ===========")
					logger.Infof(fmt.Sprintf("main: canRequest %s{%s} parameter: %s\n", metric, scope, parameter))
					canAnswer := canRequest(inverter, parameter)
					logger.Infof(fmt.Sprintf("main: %s{%s}: %v\n", metric, scope, canAnswer))
					// Test timeout
					//time.Sleep(time.Second * 5)
					c1 <- canAnswer
				}()

				select {
				case res := <-c1:
					canAnswerInt, _ := strconv.Atoi(res)
					exposed[metric].WithLabelValues(trimmedSerialNumber, scope).Set(float64(canAnswerInt))
					publishMqtt(trimmedSerialNumber, scope, metric, canAnswerInt)
				case <-time.After(1 * time.Second):
					logger.Errorf("Timeout, ignoring value")
				}

			}

		}

		// liveness check
		res, err := http.Get("http://127.0.0.1:" + listenPort + "/metrics")
		if err == nil {
			daemon.SdNotify(false, daemon.SdNotifyWatchdog)
		}
		// Read all away or else we'll run out of sockets sooner or later
		_, _ = ioutil.ReadAll(res.Body)
		defer res.Body.Close()

		logger.Infof("Sleeping...")
		time.Sleep(time.Second * 60)
	}
}
