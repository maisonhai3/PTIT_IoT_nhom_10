namespace sim { void startStdinThread(); }
void setup();
void loop();
int main() {
  sim::startStdinThread();
  setup();
  for (;;) loop();  // same as the Arduino loopTask
}
