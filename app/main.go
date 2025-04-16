package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"
)

// Ensures gofmt doesn't remove the "net" and "os" imports above (feel free to remove this!)
var _ = net.Listen
var _ = os.Exit

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	var directory string
	flag.StringVar(&directory, "directory", "/tmp", "Directory for files")
	flag.Parse()

	// Uncomment this block to pass the first stage

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()

		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go func(c net.Conn) {
			reader := bufio.NewReader(conn)
			data, err := reader.ReadString('\n')

			headers := make(map[string]string)
			for {
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(line)
				if line == "" {
					break // End of headers
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					headers[key] = value
				}
			}

			if err != nil {
				fmt.Println("Failed to read request")
			}

			parts := strings.Split(data, " ")

			url := parts[1]

			echoPath := regexp.MustCompile(`^/echo/[^/]+$`)
			filesPath := regexp.MustCompile(`^/files/[^/]+$`)

			if url == "/" {
				conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			} else if echoPath.MatchString(url) {
				responseBody := strings.TrimSpace(strings.TrimPrefix(url, "/echo/"))
				response := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
					len(responseBody),
					responseBody,
				)

				conn.Write([]byte(response))
			} else if url == "/user-agent" {
				userAgent := headers["User-Agent"]
				response := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
					len(userAgent),
					userAgent,
				)

				conn.Write([]byte(response))
			} else if filesPath.MatchString(url) {
				fileName := strings.TrimSpace(strings.TrimPrefix(url, "/files/"))
				filePath := fmt.Sprintf("%s/%s", directory, fileName)
				fmt.Println(filePath)

				// Try to open the file
				f, err := os.Open(filePath)

				// If file doesn't exist, return 404 immediately
				if errors.Is(err, syscall.ENOENT) {
					conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: application/octet-stream\r\nContent-Length: 0\r\n\r\n"))
					c.Close()
					return
				}

				// If there was some other error opening the file
				if err != nil {
					conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
					c.Close()
					return
				}

				// Make sure we close the file when done
				defer f.Close()

				// Read file contents
				b, err := os.ReadFile(filePath)
				if err != nil {
					conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
					c.Close()
					return
				}

				// Get file size
				fi, err := f.Stat()
				if err != nil {
					conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
					c.Close()
					return
				}

				size := fi.Size()
				response := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s",
					size,
					string(b),
				)
				conn.Write([]byte(response))
				c.Close()
				return
			} else {
				conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 0\r\n\r\n"))
			}

			c.Close()
		}(conn)
	}
}
