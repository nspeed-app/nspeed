function Init() {
    var conn;
    var msg = document.getElementById("msg");
    var log = document.getElementById("log");

    if (window["WebSocket"]) {
        console.log("ws on");
        conn = new WebSocket("ws://" + document.location.host + "/ws");

        conn.onopen = (e) => {
            console.log("ws opened");
            goLog("ws opened")
        };
        conn.onclose = (evt) => {
            goLog("connection closed");
        };
        conn.onmessage = (evt) => {
            var messages = evt.data.split('\n');
            for (var i = 0; i < messages.length; i++) {
                goLog("message",messages[i]);
            }
        };
        conn.onerror = (evt) => {
            console.log("WebSocket error");
        }
    } else {
        window.alert("Your browser does not support WebSockets");
        console.log("Your browser does not support WebSockets");
    }
};
