package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
)

type Method string
type ContentType string

const (
	LIST    Method = "LIST" // NewRoute duplicates into GET/POST
	POST    Method = "POST"
	GET     Method = "GET"
	PUT     Method = "PUT"
	DELETE  Method = "DELETE"
	PATCH   Method = "PATCH"
	OPTIONS Method = "OPTIONS"
)

const (
	EMPTY ContentType = ""
	JSON  ContentType = "application/json"
	TEXT  ContentType = "text/plain; charset=utf-8"
)

const (
	ROLE     = "Role"
	USER     = "Username"
	PASSWORD = "Password"
)

type ctxKey struct{}

type Route struct {
	method  Method
	regex   *regexp.Regexp
	handler http.HandlerFunc
}

var sampleSecretKey = []byte("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJSb2xlIjoiZmFsc2UiLCJVc2VybmFtZSI6ImFkbWluIiwiYXV0aG9yaXplZCI6dHJ1ZSwiZXhwIjoiMjAyMy0wMS0xNFQxOTo1MzowMy4zMDU2MTJaIn0.btpfbe-q1zJ6Cu21k3FqsXoGEGz2PiIMJltkEIK51F") // move to config

func NewServer(port int, routes []Route) error {
	listenAddr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(makeHandler(routes))
	return http.ListenAndServe(listenAddr, handler)
}

func NewRoute(method Method, pattern string, handler http.HandlerFunc) Route {
	return Route{method, regexp.MustCompile("^" + pattern + "$"), handler}
}

func makeHandler(routes []Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var allow []string

		w.Header().Set("Access-Control-Allow-Origin", "*") // TODO

		for i, route := range routes {
			if route.method == LIST {
				routes[i].method = GET
				route.method = POST
				routes = append(routes, route)
			}
		}

		for _, route := range routes {
			matches := route.regex.FindStringSubmatch(r.URL.Path)

			if len(matches) > 0 {
				if r.Method == http.MethodOptions {
					// enable Cors
					w.Header().Set("Access-Control-Allow-Methods",
						"GET,POST,PATCH,PUT,DELETE,OPTIONS")

					w.Header().Set("Access-Control-Allow-Headers",
						"access-control-allow-origin,authorization,content-type,x-requested-with")

					w.Header().Set("Access-Control-Allow-Credentials", "true")

					w.Header().Set("Access-Control-Max-Age", "3600")

					w.WriteHeader(http.StatusNoContent)
					return
				}

				if r.Method != string(route.method) {
					allow = append(allow, string(route.method))
					continue
				}

				ctx := context.WithValue(r.Context(), ctxKey{}, matches[1:])
				route.handler(w, r.WithContext(ctx))
				return
			}
		}

		if len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)

			return
		}

		http.NotFound(w, r)
	}
}

// ////////////////////////////////////////////////////////////////////////////
// Response helpers

func Respond(w http.ResponseWriter, status int, contentType ContentType, output any) {
	w.WriteHeader(status)

	if contentType != EMPTY {
		w.Header().Set("Content-Type", string(contentType))
	}

	if output != nil {
		json.NewEncoder(w).Encode(&output)
	}
}

func RespondIfFound(w http.ResponseWriter, contentType ContentType, output any) {
	if output != nil && !reflect.ValueOf(output).IsZero() {
		Respond(w, http.StatusOK, contentType, output)
		return
	}

	Respond(w, http.StatusNotFound, EMPTY, nil)
}

func ReadPayload(r *http.Request) Payload {
	payload := Payload{}
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&payload)

	return payload
}

func ReadEntity[T any](r *http.Request) T {
	var entity T
	decoder := json.NewDecoder(r.Body)
	decoder.Decode(&entity)

	return entity
}

func ReadQueryParam(r *http.Request, index int) string {
	fields := r.Context().Value(ctxKey{}).([]string)
	if len(fields) > index {
		return fields[index]
	}

	panic("Wrong route - no field")
}

// ////////////////////////////////////////////////////////////////////////////
// JWT

func Autorize(username string, role string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["exp"] = time.Now().Add(24 * time.Hour)
	claims["authorized"] = true
	claims[USER] = username
	claims[ROLE] = role
	tokenString, err := token.SignedString(sampleSecretKey)
	if err != nil {
		return "Signing Error", err
	}

	return tokenString, nil
}

func Auth(r *http.Request) (string, string, error) {
	if r.Header["Authorization"] != nil && len(r.Header["Authorization"]) == 1 && strings.Contains(r.Header["Authorization"][0], "Bearer ") {
		bearer := strings.Split(r.Header["Authorization"][0], " ")[1]
		var keyfunc jwt.Keyfunc = func(token *jwt.Token) (interface{}, error) {
			return sampleSecretKey, nil
		}
		token, _ := jwt.Parse(bearer, keyfunc)
		/*	 	if err != nil {
			log.Fatalf("Failed to parse JWT.\nError: %s", err.Error())
		}

		if !token.Valid {
			log.Fatalln("Token is not valid.")
		}

		log.Println("Token is valid.")

		if err != nil {
			return "Error Parsing Token: ", "", err
		}
		*/
		claims, ok := token.Claims.(jwt.MapClaims)
		if ok { // && token.Valid {
			username := claims[USER].(string)
			role := claims[ROLE].(string)
			return username, role, nil
		}
	}

	return "Error Parsing Token: ", "", errors.New("no token/error parsing token")
}
