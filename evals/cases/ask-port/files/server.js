const http = require("http");

const PORT = 8123;

const server = http.createServer((req, res) => {
  res.end("ok\n");
});

server.listen(PORT, () => console.log("listening"));
