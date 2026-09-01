package store

import "testing"

/*
 * A conversation can be named and removed.
 *
 * Titles are taken from the opening message, which is a fair guess and often a
 * poor name — one that began "can you hear me" is not about that, and twenty
 * such rows cannot be told apart.
 */
func TestAConversationCanBeRenamedAndDeleted(t *testing.T) {
	db := open(t)

	id, err := db.NewConversation("can you hear me")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(id, "user", "", "", "can you hear me"); err != nil {
		t.Fatal(err)
	}

	if err := db.RenameConversation(id, "Finding my projects"); err != nil {
		t.Fatalf("renaming: %v", err)
	}

	recent, err := db.RecentConversations(10)
	if err != nil {
		t.Fatal(err)
	}

	var found bool

	for _, c := range recent {
		if c.ID == id {
			found = true

			if c.Title != "Finding my projects" {
				t.Errorf("the new name is not shown: %q", c.Title)
			}
		}
	}

	if !found {
		t.Fatal("the conversation vanished from the list after being renamed")
	}

	// An empty name is not a name, and would leave a row nothing identifies.
	if err := db.RenameConversation(id, "   "); err == nil {
		t.Error("a blank name was accepted")
	}

	if err := db.DeleteConversation(id); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	if exists, _ := db.ConversationExists(id); exists {
		t.Error("it is still there after being deleted")
	}

	// And deleting one that is gone says so rather than reporting success.
	if err := db.DeleteConversation(id); err == nil {
		t.Error("deleting a conversation that does not exist was reported as working")
	}
}
