package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"syscall"
)

var compressionSchemes = []string{"gzip"}

func main() {
	fmt.Println("Logs from your program will appear here!")

	var directory string
	flag.StringVar(&directory, "directory", "/tmp", "Directory for files")
	flag.Parse()

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

		go handleConnection(conn, directory)
	}
}

func handleConnection(conn net.Conn, directory string) {
	defer conn.Close() // Ensure connection is always closed when function returns

	reader := bufio.NewReader(conn)

	// Read the request line
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Failed to read request:", err)
		conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}

	parts := strings.Split(strings.TrimSpace(requestLine), " ")
	if len(parts) < 3 {
		fmt.Println("Invalid request format")
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	method := parts[0]
	url := parts[1]

	// Parse headers
	headers := make(map[string]string)
	contentLength := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading headers:", err)
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			headers[key] = value

			if strings.ToLower(key) == "content-length" {
				fmt.Sscanf(value, "%d", &contentLength)
			}
		}
	}

	// Read body if needed (for POST requests)
	var body []byte
	if method == "POST" && contentLength > 0 {
		body = make([]byte, contentLength)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			fmt.Println("Error reading request body:", err)
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}
	}

	echoPath := regexp.MustCompile(`^/echo/[^/]+$`)
	filesPath := regexp.MustCompile(`^/files/[^/]+$`)

	if url == "/" {
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	} else if echoPath.MatchString(url) {
		// Support compression
		compressResponse := false
		responseBody := strings.TrimSpace(strings.TrimPrefix(url, "/echo/"))

		if value, ok := headers["Accept-Encoding"]; ok {
			if value == "gzip" {
				compressResponse = true
			}
		}

		var response string
		if compressResponse {

			var compressedResponse bytes.Buffer
			w := gzip.NewWriter(&compressedResponse)
			w.Write([]byte(responseBody))
			w.Close()

			response = fmt.Sprintf(
				"HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
				len(compressedResponse.Bytes()),
				compressedResponse.Bytes(),
			)
		} else {
			response = fmt.Sprintf(
				"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
				len(responseBody),
				responseBody,
			)
		}

		conn.Write([]byte(response))
	} else if url == "/user-agent" {
		userAgent := headers["User-Agent"]
		response := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
			len(userAgent),
			userAgent,
		)
		conn.Write([]byte(response))
	} else if filesPath.MatchString(url) && method == "GET" {
		fileName := strings.TrimSpace(strings.TrimPrefix(url, "/files/"))
		filePath := fmt.Sprintf("%s/%s", directory, fileName)

		// Try to open the file
		f, err := os.Open(filePath)

		// If file doesn't exist, return 404
		if errors.Is(err, syscall.ENOENT) {
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: application/octet-stream\r\nContent-Length: 0\r\n\r\n"))
			return
		}

		// If there was some other error opening the file
		if err != nil {
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		// Make sure we close the file when done
		defer f.Close()

		// Read file contents
		b, err := os.ReadFile(filePath)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		// Get file size
		fi, err := f.Stat()
		if err != nil {
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		size := fi.Size()
		response := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
			size,
		)
		conn.Write([]byte(response))
		conn.Write(b)
	} else if filesPath.MatchString(url) && method == "POST" {
		fileName := strings.TrimSpace(strings.TrimPrefix(url, "/files/"))
		filePath := fmt.Sprintf("%s/%s", directory, fileName)

		// Write the file to the directory
		err := os.WriteFile(filePath, body, 0644)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}

		conn.Write([]byte("HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\n"))
	} else {
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 0\r\n\r\n"))
	}
}
