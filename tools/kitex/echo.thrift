namespace go kitexpb

struct EchoRequest {
  1: string message
}

struct EchoResponse {
  1: string message
}

service EchoService {
  EchoResponse Echo(1: EchoRequest req)
}
