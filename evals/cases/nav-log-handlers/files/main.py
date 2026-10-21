from handlers.users import handle_get_user
from handlers.orders import handle_create_order

print(handle_get_user({"id": 1}), handle_create_order({"item": "pen"}))
