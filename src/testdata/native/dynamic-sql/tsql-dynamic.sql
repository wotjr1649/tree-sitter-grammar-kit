-- Owned dynamic SQL location fixture for dynamic-sql-r1 (S05-A21).
-- Expected facts and non-facts are registered in expected.json.
SELECT 1 AS anchor;
DECLARE @sql nvarchar(max) = N'SELECT 1 AS a';
DECLARE @tbl sysname = N'dbo.t';
DECLARE @proc sysname = N'dbo.p';
DECLARE @rc int;
-- EXEC_PAREN with each argument kind.
EXEC (N'SELECT 1 AS a');
EXEC ('SELECT 2 AS a');
EXECUTE (@sql);
EXEC (N'SELECT * FROM ' + @tbl + N' WHERE 1 = 1');
-- EXEC_PAREN_AT; arguments after the first are pass-through parameters.
EXEC (N'SELECT 3 AS a') AT LinkedSrv;
EXEC (@sql, 10, @tbl) AT LinkedSrv;
-- SP_EXECUTESQL with name normalization and each argument kind.
EXEC sp_executesql N'SELECT 4 AS a';
EXEC sys.sp_executesql @sql;
EXEC [sp_executesql] sp_who;
EXEC @rc = SP_EXECUTESQL @stmt = N'SELECT @p AS a', @params = N'@p int', @p = 5;
EXECUTE sp_executesql NULL;
-- AS USER / AS LOGIN expressions are excluded arguments.
EXEC (N'SELECT USER_NAME() AS u') AS USER = 'app_user';
EXEC ('SELECT SUSER_NAME() AS l') AS LOGIN = 'app_login';
-- EXEC @module_var is not dynamic SQL.
EXEC @proc;
GO
-- Known miss: procedure call as the first statement of a batch without EXEC.
sp_executesql N'SELECT 5 AS a';
GO
-- Known miss: AT DATA_SOURCE.
EXEC (N'SELECT 6 AS a') AT DATA_SOURCE RemoteDs;
GO
-- Known miss: WITH RESULT SETS.
EXEC sp_executesql N'SELECT 7 AS a' WITH RESULT SETS ((a int));
GO
