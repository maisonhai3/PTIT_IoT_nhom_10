// Host stub of the Arduino WiFi library: "WiFi" is always up (unless the driver says
// otherwise) and WiFiClient is a plain POSIX TCP client.
#pragma once
#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <poll.h>
#include <sys/socket.h>
#include <unistd.h>
#include <fcntl.h>
#include <deque>
#include "Arduino.h"
#include "Client.h"
#include "IPAddress.h"

namespace sim { bool stallConnect(); int wifiEpoch(); }

typedef enum { WL_IDLE_STATUS = 0, WL_NO_SSID_AVAIL = 1, WL_CONNECTED = 3, WL_CONNECT_FAILED = 4, WL_DISCONNECTED = 6 } wl_status_t;
typedef enum { WIFI_MODE_NULL = 0, WIFI_STA = 1 } wifi_mode_t;

class WiFiClass {
 public:
  void persistent(bool) {}
  bool mode(wifi_mode_t) { return true; }
  bool setHostname(const char*) { return true; }
  bool setSleep(bool) { return true; }
  void setAutoReconnect(bool) {}
  wl_status_t begin(const char* ssid, const char* pass);
  bool disconnect(bool wifioff = false, bool eraseap = false);
  wl_status_t status();
  IPAddress localIP() { return IPAddress(192, 168, 1, 50); }
  int8_t RSSI() { return -58; }
};
extern WiFiClass WiFi;

class WiFiClient : public Client {
 public:
  ~WiFiClient() { stop(); }
  int connect(IPAddress, uint16_t) override { return 0; }
  int connect(const char* host, uint16_t port) override {
    stop();
    if (sim::stallConnect()) return 0;  // emulate a black-holed broker: block, then fail
    struct addrinfo hints;
    memset(&hints, 0, sizeof hints);
    hints.ai_family = AF_INET;
    hints.ai_socktype = SOCK_STREAM;
    struct addrinfo* res = nullptr;
    char portStr[8];
    snprintf(portStr, sizeof portStr, "%u", port);
    if (getaddrinfo(host, portStr, &hints, &res) != 0 || res == nullptr) return 0;
    fd_ = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
    if (fd_ < 0) { freeaddrinfo(res); return 0; }
    fcntl(fd_, F_SETFL, fcntl(fd_, F_GETFL, 0) | O_NONBLOCK);
    int rc = ::connect(fd_, res->ai_addr, res->ai_addrlen);
    freeaddrinfo(res);
    if (rc < 0 && errno == EINPROGRESS) {
      struct pollfd p = {fd_, POLLOUT, 0};
      if (poll(&p, 1, 3000) <= 0) { stop(); return 0; }
      int err = 0;
      socklen_t len = sizeof err;
      getsockopt(fd_, SOL_SOCKET, SO_ERROR, &err, &len);
      if (err != 0) { stop(); return 0; }
    } else if (rc < 0) {
      stop();
      return 0;
    }
    epoch_ = sim::wifiEpoch();
    return 1;
  }
  size_t write(uint8_t b) override { return write(&b, 1); }
  size_t write(const uint8_t* buf, size_t size) override {
    if (fd_ < 0 || dead()) return 0;
    size_t sent = 0;
    while (sent < size) {
      ssize_t n = ::send(fd_, buf + sent, size - sent, MSG_NOSIGNAL);
      if (n > 0) { sent += n; continue; }
      if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
        struct pollfd p = {fd_, POLLOUT, 0};
        poll(&p, 1, 1000);
        continue;
      }
      closed_ = true;
      break;
    }
    return sent;
  }
  int available() override { fill(); return static_cast<int>(rx_.size()); }
  int read() override {
    fill();
    if (rx_.empty()) return -1;
    int b = rx_.front();
    rx_.pop_front();
    return b;
  }
  int read(uint8_t* buf, size_t size) override {
    size_t n = 0;
    while (n < size) { int b = read(); if (b < 0) break; buf[n++] = static_cast<uint8_t>(b); }
    return static_cast<int>(n);
  }
  int peek() override { fill(); return rx_.empty() ? -1 : rx_.front(); }
  void flush() override {}
  void stop() override {
    if (fd_ >= 0) ::close(fd_);
    fd_ = -1;
    closed_ = false;
    rx_.clear();
  }
  uint8_t connected() override { fill(); return fd_ >= 0 && !dead() ? 1 : (rx_.empty() ? 0 : 1); }
  operator bool() override { return connected(); }

 private:
  bool dead() {
    if (epoch_ != sim::wifiEpoch()) closed_ = true;  // simulated WiFi dropped: socket is gone
    return closed_;
  }
  void fill() {
    if (fd_ < 0 || dead()) return;
    uint8_t tmp[512];
    for (;;) {
      ssize_t n = ::recv(fd_, tmp, sizeof tmp, MSG_DONTWAIT);
      if (n > 0) { for (ssize_t i = 0; i < n; ++i) rx_.push_back(tmp[i]); continue; }
      if (n == 0) { closed_ = true; return; }
      return;  // EAGAIN
    }
  }
  int fd_ = -1;
  int epoch_ = 0;
  bool closed_ = false;
  std::deque<uint8_t> rx_;
};
