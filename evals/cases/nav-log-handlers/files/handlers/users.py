def handle_list_users(request):
    return {"users": ["ada", "bob"]}


def handle_get_user(request):
    return {"user": request.get("id")}
