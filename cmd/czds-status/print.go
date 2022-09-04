package main

import (
	"fmt"
	"time"

	"github.com/lanrat/czds"
)

func printRequestInfo(info *czds.RequestsInfo) {
	fmt.Printf("ID:\t%s\n", info.RequestID)
	fmt.Printf("TLD:\t%s (%s)\n", info.TLD.TLD, info.TLD.ULabel)
	fmt.Printf("Status:\t%s\n", info.Status)
	fmt.Printf("Created:\t%s\n", info.Created.Format(time.ANSIC))
	fmt.Printf("Updated:\t%s\n", info.LastUpdated.Format(time.ANSIC))
	fmt.Printf("Expires:\t%s\n", expiredTime(info.Expired))
	fmt.Printf("AutoRenew:\t%t\n", info.AutoRenew)
	fmt.Printf("Extensible:\t%t\n", info.Extensible)
	fmt.Printf("ExtensionInProcess:\t%t\n", info.ExtensionInProcess)
	fmt.Printf("Cancellable:\t%t\n", info.Cancellable)
	fmt.Printf("Request IP:\t%s\n", info.RequestIP)
	fmt.Println("FTP IPs:\t", info.FtpIps)
	fmt.Printf("Reason:\t%s\n", info.Reason)
	fmt.Printf("History:\n")
	for _, event := range info.History {
		fmt.Printf("\t%s\t%s\n", event.Timestamp.Format(time.ANSIC), event.Action)
	}
}

func printRequest(request czds.Request) {
	fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\n",
		request.TLD,
		request.RequestID,
		request.ULabel,
		request.Status,
		request.Created.Format(time.ANSIC),
		request.LastUpdated.Format(time.ANSIC),
		expiredTime(request.Expired),
		request.SFTP)
}

func printHeader() {
	fmt.Printf("TLD\tID\tUnicodeTLD\tStatus\tCreated\tUpdated\tExpires\tSFTP\n")
}
