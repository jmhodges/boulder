These `.sql` files define the table layout, indicies, relationships, and users default to Boulder. Implementors should use these as starting points for their own configuration.

The currently supported database is MariaDB 5.5.

    mysql -u root -e "create database boulder_test; create database boulder_development; grant all privileges on boulder_test.* to 'boulder'@'localhost';"

