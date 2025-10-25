import { useState, useEffect } from "react";

export default function App() {
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState([]);
  const [socket, setSocket] = useState(null);

  useEffect(() => {
    const backendUrl =
      import.meta.env.PROD
        ? "wss://go-backend-dqcl.onrender.com/ws" // продакшен
        : "ws://localhost:8080/ws"; // локально

    const ws = new WebSocket(backendUrl);
    ws.onopen = () => console.log("✅ WebSocket connected");
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data && data.digits && data.time) {
          setMessages((prev) => [data, ...prev]);
        }
      } catch (err) {
        console.error("Ошибка парсинга сообщения:", err);
      }
    };

    ws.onclose = () => console.log("❌ WebSocket disconnected");
    setSocket(ws);
    return () => ws.close();
  }, []);

  const sendDigits = async () => {
    if (!input.match(/^\d{4}$/)) {
      alert("Введите ровно 4 цифры!");
      return;
    }
    try {
      await fetch("https://go-backend-dqcl.onrender.com/api/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ digits: input }),
      });
      setInput("");
    } catch {
      alert("Ошибка при отправке");
    }
  };

  // Группировка по минутам
  const grouped = messages.reduce((acc, msg) => {
    if (!msg || !msg.time) return acc; // 👈 защита от пустых сообщений

    const minute = msg.time.slice(0, 5);
    if (!acc[minute]) acc[minute] = [];
    acc[minute].push(msg.digits);
    return acc;
  }, {});


  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-100 to-slate-200 flex flex-col items-center justify-center p-6">
      <div className="w-full max-w-md bg-white rounded-2xl shadow-lg p-6 space-y-6">
        <h1 className="text-2xl font-bold text-center text-slate-800">
          Введите номер заказа
        </h1>

        <div className="flex items-center space-x-2">
          <input
            type="text"
            maxLength={4}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Например: 1234"
            className="flex-1 border border-slate-300 rounded-xl px-4 py-2 text-lg text-center focus:outline-none focus:ring-2 focus:ring-blue-400"
          />
          <button
            onClick={sendDigits}
            className="px-4 py-2 bg-blue-600 text-white font-semibold rounded-xl hover:bg-blue-900 active:scale-95 transition"
          >
            Отправить
          </button>
        </div>

        <div className="space-y-4 max-h-[400px] overflow-y-auto border-t border-slate-200 pt-4">
          {Object.keys(grouped).length === 0 ? (
            <p className="text-center text-slate-500">Нет данных</p>
          ) : (
            Object.entries(grouped).map(([minute, values]) => (
              <div
                key={minute}
                className="bg-slate-50 border border-slate-200 rounded-xl p-3"
              >
                <div className="text-sm font-semibold text-slate-600 mb-1">
                  {minute}
                </div>
                <div className="flex flex-wrap gap-2">
                  {values.map((v, i) => (
                    <span
                      key={i}
                      className="px-3 py-1 bg-blue-100 text-blue-700 font-mono rounded-lg"
                    >
                      {v}
                    </span>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
