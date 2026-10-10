const ws = new WebSocket('ws://localhost:8080');

ws.onopen = function() {
  console.log('connected');
  ws.send('Hello, world!');
};

ws.onmessage = function(event) {
  console.log('message: ', event.data);
};

ws.onclose = function() {
  console.log('disconnected');
};

window.onbeforeunload = function() {
  ws.close();
};
